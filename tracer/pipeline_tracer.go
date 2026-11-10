package tracer

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Chaintable/pipeline/leader"
	"github.com/Chaintable/pipeline/metrics"
	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var _ vm.EVMLogger = (*PipelineTracer)(nil)

// PipelineTracer traces blocks while the WEMIX writer imports them and
// publishes the artifacts that trace_debankBlock used to hand to the ETL: the
// block file and its validation go to the ChainTable bucket, the header and
// state diff go to the NodeX bucket, and canonical head changes go to Kafka.
//
// Its output follows RPCTracer, so consumers see the same data whichever path
// produced it. The writer calls it from the block import path only, which
// holds the chain lock, so the in-flight block lives in plain fields.
type PipelineTracer struct {
	config     pipelineTracerConfig
	callTracer *callTracer
	block      *ExtraInfo

	// published and firstPublished record the first block this process
	// published, to flag notifications that announce older blocks.
	published      bool
	firstPublished uint64
}

type pipelineTracerConfig struct {
	Region               string   `json:"region"`
	NodeXBucket          string   `json:"node_x_bucket"`
	ChainTableBucket     string   `json:"chain_table_bucket"`
	Brokers              []string `json:"brokers"`
	Topic                string   `json:"topic"`
	S3TempDir            string   `json:"s3_temp_dir"`
	IsBackup             *bool    `json:"is_backup"` // nil = auto (use etcd), false = leader in fixed mode, true = backup in fixed mode
	EnablePreStateTracer bool     `json:"enable_prestate_tracer"`
	Version              string   `json:"version"`

	// Auto failover configurations
	EtcdEndpoints []string `json:"etcd_endpoints"`
	ElectionKey   string   `json:"election_key"`
	NodeID        string   `json:"node_id"`      // default to hostname
	GracePeriod   int      `json:"grace_period"` // default to 10 seconds, unit is second

	// Writer node registry configurations
	WriterRegistryTTL int64 `json:"writer_registry_ttl"` // TTL for writer node registration in seconds, default 10
}

func (config *pipelineTracerConfig) fillDefaultValues() error {
	if config.Region == "" || config.NodeXBucket == "" || config.ChainTableBucket == "" || len(config.Brokers) == 0 {
		return errors.New("region, node_x_bucket, chain_table_bucket and brokers are required")
	}
	if config.IsBackup == nil && len(config.EtcdEndpoints) == 0 {
		return errors.New("is_backup is unset and etcd_endpoints is empty: set is_backup for manual mode or etcd_endpoints for failover mode")
	}
	if config.IsBackup != nil && len(config.EtcdEndpoints) > 0 {
		return errors.New("is_backup and etcd_endpoints are mutually exclusive")
	}
	if config.NodeID == "" {
		hostname := os.Getenv("HOSTNAME")
		if hostname == "" {
			var err error
			if hostname, err = os.Hostname(); err != nil {
				return fmt.Errorf("failed to resolve node id: %w", err)
			}
		}
		config.NodeID = hostname
	}
	if config.GracePeriod == 0 {
		config.GracePeriod = 10
	}
	if config.WriterRegistryTTL == 0 {
		config.WriterRegistryTTL = 10
	}
	return nil
}

// NewPipelineTracer parses the --vmtrace.jsonconfig value.
func NewPipelineTracer(cfg json.RawMessage) (*PipelineTracer, error) {
	var config pipelineTracerConfig
	if cfg != nil {
		if err := json.Unmarshal(cfg, &config); err != nil {
			return nil, fmt.Errorf("failed to parse pipeline tracer config: %w", err)
		}
	}
	if err := config.fillDefaultValues(); err != nil {
		return nil, fmt.Errorf("invalid pipeline tracer config: %w", err)
	}
	log.Info("NewPipelineTracer", "config", config)
	return &PipelineTracer{config: config}, nil
}

// OnBlockchainInit connects the S3 and Kafka pushers and joins the writer
// leader election. It runs once, after the chain has been loaded.
func (t *PipelineTracer) OnBlockchainInit(chainConfig *params.ChainConfig) error {
	log.Info("Init pipeline", "chainID", chainConfig.ChainID, "config", t.config)

	if t.config.ElectionKey == "" {
		if t.config.Version == "" {
			t.config.ElectionKey = fmt.Sprintf("%s/writers/leader", chainConfig.ChainID.String())
		} else {
			t.config.ElectionKey = fmt.Sprintf("%s/%s/writers/leader", chainConfig.ChainID.String(), t.config.Version)
		}
	}
	if t.config.Topic == "" {
		if t.config.Version == "" {
			t.config.Topic = fmt.Sprintf("nodex_pipeline_%d", chainConfig.ChainID)
		} else {
			t.config.Topic = fmt.Sprintf("nodex_pipeline_%d_%s", chainConfig.ChainID, t.config.Version)
		}
	}

	if err := InitPipeline(t.config.Region, t.config.NodeXBucket, t.config.ChainTableBucket,
		t.config.Brokers, t.config.Topic, chainConfig.ChainID.String(), t.config.Version,
		t.config.S3TempDir); err != nil {
		return fmt.Errorf("failed to init pipeline: %w", err)
	}

	// Writer registry is always enabled when etcd endpoints are configured.
	var writerConfig *WriterRegistryConfig
	if len(t.config.EtcdEndpoints) > 0 {
		writerConfig = &WriterRegistryConfig{
			TTL:              t.config.WriterRegistryTTL,
			NodeXBucket:      t.config.NodeXBucket,
			ChainTableBucket: t.config.ChainTableBucket,
			Region:           t.config.Region,
			Brokers:          t.config.Brokers,
			Topic:            t.config.Topic,
		}
	}
	if err := SetupLeaderElection(t.config.EtcdEndpoints, t.config.ElectionKey,
		t.config.NodeID, t.config.Version, t.config.IsBackup, t.config.GracePeriod, writerConfig); err != nil {
		return fmt.Errorf("failed to setup leader election: %w", err)
	}

	// Upload workers start after the election so they know the node's role.
	if err := NodeXPusher.StartUploadWork(); err != nil {
		return fmt.Errorf("failed to start NodeX upload work: %w", err)
	}
	if err := ChainTableBucketPusher.StartUploadWork(); err != nil {
		return fmt.Errorf("failed to start ChainTable upload work: %w", err)
	}

	metrics.NodeInfo.Update(map[string]string{
		"chain_id": chainConfig.ChainID.String(),
		"role":     "writer",
	})
	return nil
}

// OnClose leaves the election and stops the pushers.
func (t *PipelineTracer) OnClose() {
	if WriterRegistry != nil {
		if err := WriterRegistry.UnregisterNode(); err != nil {
			log.Error("Failed to unregister writer node during shutdown", "err", err)
		} else {
			log.Info("Writer node unregistered during shutdown")
		}
	}
	if LeaderManager != nil {
		LeaderManager.Close()
		leader.GlobalManager = nil
	}
	if NodeXPusher != nil {
		NodeXPusher.Close()
	}
	if ChainTableBucketPusher != nil {
		ChainTableBucketPusher.Close()
	}
}

// OnBlockStart starts collecting a block that is about to be executed.
func (t *PipelineTracer) OnBlockStart(block *types.Block) {
	t.callTracer = nil
	t.block = &ExtraInfo{
		BlockNumber:     block.NumberU64(),
		BlockHash:       block.Hash(),
		BlockHeader:     util.BuildPilelineBlockHeader(block),
		BlockFile:       newBlockFile(block),
		BlockStartTime:  time.Now(),
		ChangeContracts: make(map[common.Address]struct{}),
	}
}

// OnBlockEnd publishes the block once it has been executed and validated. The
// writer calls it before storing the block, so a crash cannot leave a stored
// block unpublished. A failed block publishes nothing.
func (t *PipelineTracer) OnBlockEnd(blockErr error) {
	block := t.block
	t.block, t.callTracer = nil, nil
	if block == nil {
		return
	}
	defer metrics.BlockProcessTimer.UpdateSince(block.BlockStartTime)

	if blockErr != nil {
		log.Error("Skip publishing failed block", "number", block.BlockNumber, "hash", block.BlockHash, "err", blockErr)
		return
	}
	if !block.Committed {
		// The import path reports the state changes before writing the block;
		// publishing without them would ship a block with no state diff.
		log.Crit("Pipeline block ended without state changes", "number", block.BlockNumber, "hash", block.BlockHash)
	}
	publishBlock(block)
	if !t.published {
		t.published, t.firstPublished = true, block.BlockNumber
	}
}

// vm.EVMLogger interface implementation, forwarded to the current transaction.

func (t *PipelineTracer) CaptureTxStart(gasLimit uint64) {
	if t.callTracer != nil {
		t.callTracer.CaptureTxStart(gasLimit)
	}
}

func (t *PipelineTracer) CaptureTxEnd(restGas uint64) {
	if t.callTracer != nil {
		t.callTracer.CaptureTxEnd(restGas)
	}
}

func (t *PipelineTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	if t.callTracer != nil {
		t.callTracer.CaptureStart(env, from, to, create, input, gas, value)
	}
}

func (t *PipelineTracer) CaptureEnd(output []byte, gasUsed uint64, d time.Duration, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureEnd(output, gasUsed, d, err)
	}
}

func (t *PipelineTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.callTracer != nil {
		t.callTracer.CaptureEnter(typ, from, to, input, gas, value)
	}
}

func (t *PipelineTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureExit(output, gasUsed, err)
	}
}

func (t *PipelineTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureState(pc, op, gas, cost, scope, rData, depth, err)
	}
}

func (t *PipelineTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureFault(pc, op, gas, cost, scope, depth, err)
	}
}

// OnTxStart starts tracing a transaction of the current block.
func (t *PipelineTracer) OnTxStart(tx *types.Transaction, from common.Address) {
	if t.block == nil {
		return
	}
	t.callTracer = newCallTracerRaw(t.block.ChangeContracts, t.block.BlockFile)
	t.callTracer.OnTxStart(tx, from)
	t.block.Tx = tx
	t.block.From = from
	t.block.TxStartTime = time.Now()
}

// OnTxEnd records the transaction. err is set when the transaction could not
// be applied, which also fails the block.
func (t *PipelineTracer) OnTxEnd(receipt *types.Receipt, err error) {
	if t.block == nil {
		return
	}
	defer metrics.BlockTxExecutionTimer.UpdateSince(t.block.TxStartTime)
	if t.callTracer != nil {
		t.callTracer.OnTxEnd(receipt, err)
	}
	t.callTracer = nil
	if err != nil || receipt == nil {
		return
	}
	baseFee := t.block.BlockHeader.BaseFeePerGas.ToInt()
	tx := util.BuildPipelineTransaction(t.block.Tx, receipt, t.block.From, baseFee)
	t.block.BlockFile.Txs = append(t.block.BlockFile.Txs, tx)
}

// OnLog records a log emitted by the current transaction.
func (t *PipelineTracer) OnLog(log *types.Log) {
	if t.callTracer != nil {
		t.callTracer.OnLog(log)
	}
}

// OnCommit receives the block's state changes after its state root has been
// validated, in the same shape statedb.Output returns them.
func (t *PipelineTracer) OnCommit(parentRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storages map[common.Hash]map[common.Hash][]byte, codes map[common.Hash][]byte) {
	if t.block == nil {
		return
	}
	// Same as RPCTracer.GetOutPut: a block that leaves the root unchanged has
	// no state diff.
	if parentRoot != root {
		t.block.BlockDiff = stateUpdateToStateDiff(parentRoot, root, destructs, accounts, storages, codes)
	}
	for addr := range t.block.ChangeContracts {
		t.block.BlockFile.StorageContracts = append(t.block.BlockFile.StorageContracts, strings.ToLower(addr.Hex()))
	}
	t.block.Committed = true
}

// OnGenesisBlock publishes the genesis block when the chain starts from it and
// nothing has been published yet.
func (t *PipelineTracer) OnGenesisBlock(block *types.Block, alloc core.GenesisAlloc) {
	if NodeXPusher.LastBlockNotice != nil {
		return
	}
	header, diff, blockFile := genesisArtifacts(block, alloc)
	publishBlock(&ExtraInfo{
		BlockNumber: block.NumberU64(),
		BlockHash:   block.Hash(),
		BlockHeader: header,
		BlockDiff:   diff,
		BlockFile:   blockFile,
	})
	change := &ptypes.BlockChangeNotification{
		ChangeType: 1,
		NewBlocks:  []ptypes.BlockContext{blockContext(block.Header())},
	}
	if err := NodeXPusher.PushBlockChangeNotification(change); err != nil {
		log.Crit("Failed to push genesis block change notification", "err", err)
	}
	log.Info("Published genesis block", "hash", block.Hash(), "txs", len(blockFile.Txs), "traces", len(blockFile.Traces))
}

// PushHead announces head as the new canonical head. It is a no-op on backup
// writers and when the head did not move past the last announced block.
func (t *PipelineTracer) PushHead(head *types.Header, getHeader func(common.Hash) *types.Header) error {
	if NodeXPusher == nil || leader.GlobalManager == nil || !leader.GlobalManager.IsLeader() {
		return nil
	}
	last := NodeXPusher.LastPushedBlock()
	if last == nil {
		log.Warn("Pipeline has no previously published block", "head", head.Number, "hash", head.Hash())
		return nil
	}
	change, err := blockChangeForHead(*last, head, getHeader)
	if err != nil || change == nil {
		return err
	}
	if t.published && change.NewBlocks[0].BlockNumber < t.firstPublished {
		log.Warn("Announcing blocks this process did not publish", "from", change.NewBlocks[0].BlockNumber, "firstPublished", t.firstPublished)
	}
	start := time.Now()
	if err := NodeXPusher.PushBlockChangeNotification(change); err != nil {
		return err
	}
	log.Info("Pushed pipeline head", "number", head.Number, "hash", head.Hash(), "newBlocks", len(change.NewBlocks), "dropBlocks", len(change.DropBlocks), "elapsed", common.PrettyDuration(time.Since(start)))
	return nil
}

// blockChangeForHead builds the notification that moves consumers from last to
// head, or returns nil when there is nothing to announce. Rewinds below the
// last announced height are not announced.
func blockChangeForHead(last ptypes.BlockContext, head *types.Header, getHeader func(common.Hash) *types.Header) (*ptypes.BlockChangeNotification, error) {
	if last.BlockNumber > head.Number.Uint64() {
		return nil, nil
	}
	dropBlocks, newBlocks, err := commonAncestor(last, blockContext(head), getHeader)
	if err != nil {
		return nil, err
	}
	if len(newBlocks) == 0 {
		return nil, nil
	}
	change := &ptypes.BlockChangeNotification{ChangeType: 1, NewBlocks: newBlocks}
	if len(dropBlocks) > 0 {
		change.ChangeType = 2
		change.DropBlocks = dropBlocks
	}
	return change, nil
}

// commonAncestor walks a and b back to their common ancestor and returns the
// blocks to drop from a and to add from b, both in ascending height.
func commonAncestor(a, b ptypes.BlockContext, getHeader func(common.Hash) *types.Header) ([]ptypes.BlockContext, []ptypes.BlockContext, error) {
	var dropBlocks, newBlocks []ptypes.BlockContext
	parent := func(block ptypes.BlockContext) (ptypes.BlockContext, error) {
		header := getHeader(block.ParentHash)
		if header == nil {
			return ptypes.BlockContext{}, fmt.Errorf("pipeline parent header %s of block %d not found", block.ParentHash, block.BlockNumber)
		}
		return blockContext(header), nil
	}
	var err error
	for a.BlockNumber > b.BlockNumber {
		dropBlocks = append(dropBlocks, a)
		if a, err = parent(a); err != nil {
			return nil, nil, err
		}
	}
	for b.BlockNumber > a.BlockNumber {
		newBlocks = append(newBlocks, b)
		if b, err = parent(b); err != nil {
			return nil, nil, err
		}
	}
	for a.Hash != b.Hash {
		dropBlocks = append(dropBlocks, a)
		newBlocks = append(newBlocks, b)
		if a, err = parent(a); err != nil {
			return nil, nil, err
		}
		if b, err = parent(b); err != nil {
			return nil, nil, err
		}
	}
	slices.Reverse(dropBlocks)
	slices.Reverse(newBlocks)
	return dropBlocks, newBlocks, nil
}

func blockContext(header *types.Header) ptypes.BlockContext {
	return ptypes.BlockContext{
		BlockNumber: header.Number.Uint64(),
		Hash:        header.Hash(),
		ParentHash:  header.ParentHash,
		Timestamp:   header.Time,
	}
}

func newBlockFile(block *types.Block) *ptypes.BlockFile {
	return &ptypes.BlockFile{
		Block:            util.BuildPipelineBlock(block),
		Events:           make([]ptypes.Event, 0),
		Txs:              make([]ptypes.Transaction, 0),
		Traces:           make([]ptypes.Trace, 0),
		ErrorEvents:      make([]ptypes.Event, 0),
		ErrorTraces:      make([]ptypes.Trace, 0),
		StorageContracts: make([]string, 0),
	}
}

// publishBlock uploads the block's artifacts in parallel and stops the node if
// any upload fails, so a block is never announced without its files.
func publishBlock(block *ExtraInfo) {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	upload := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	upload(func() error { return uploadBlockHeader(block.BlockHeader) })
	if block.BlockDiff != nil {
		upload(func() error { return uploadBlockDiff(block.BlockDiff) })
	}
	upload(func() error { return uploadBlockFile(block.BlockFile) })
	upload(func() error { return uploadblockFileValidation(block.BlockFile) })
	wg.Wait()

	if len(errs) > 0 {
		for _, err := range errs {
			log.Error("Upload error", "number", block.BlockNumber, "hash", block.BlockHash, "err", err)
		}
		log.Crit("One or more uploads failed", "number", block.BlockNumber, "hash", block.BlockHash)
	}
	log.Info("Upload block", "number", block.BlockNumber, "hash", block.BlockHash)
	metrics.LatestUploadedBlockNumber.Update(int64(block.BlockNumber))
}
