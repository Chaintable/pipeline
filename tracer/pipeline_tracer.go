package tracer

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/XinFinOrg/XDPoSChain/common"
	"github.com/XinFinOrg/XDPoSChain/core/types"
	"github.com/XinFinOrg/XDPoSChain/core/vm"
	"github.com/XinFinOrg/XDPoSChain/log"
	"github.com/XinFinOrg/XDPoSChain/params"

	"github.com/Chaintable/pipeline/failover"
	"github.com/Chaintable/pipeline/metrics"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
)

// 需要上传3种data
// 1. block
// 2. state diff
// 3. block file

var _ vm.EVMLogger = (*PipelineTracer)(nil)

type PipelineTracer struct {
	config     pipelineTracerConfig
	callTracer *callTracer
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
	LeaderKey     string   `json:"leader_key"`     // default to <chain_id>/[<version>/]writers/leader
	NodeID        string   `json:"node_id"`        // default to hostname
	LockTTL       int64    `json:"lock_ttl"`       // writeLock 租约时长（秒），默认 20
	CheckInterval int      `json:"check_interval"` // 指派 key 的轮询间隔（秒），默认 5
}

func (config *pipelineTracerConfig) fillDefaultValues() {
	if config.IsBackup == nil && len(config.EtcdEndpoints) == 0 {
		log.Crit("IsBackup is nil and etcd endpoints is empty, please set IsBackup to true(manual mode) or add etcd endpoints(auto mode)")
	}
	if config.IsBackup != nil && len(config.EtcdEndpoints) > 0 {
		log.Crit("IsBackup is not nil and etcd endpoints is not empty, please set IsBackup to nil(auto mode) or remove etcd endpoints(manual mode)")
	}
	if config.NodeID == "" {
		// 先尝试从环境变量 HOSTNAME 读取
		if hostname := os.Getenv("HOSTNAME"); hostname != "" {
			config.NodeID = hostname
		} else {
			// 如果环境变量不存在，则使用系统主机名
			hostname, err := os.Hostname()
			if err != nil {
				log.Crit("Failed to get hostname", "err", err)
			}
			config.NodeID = hostname
		}
	}
	if config.LockTTL <= 0 {
		config.LockTTL = 20
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 5
	}
}

func NewPipelineTracer(cfg json.RawMessage) (*PipelineTracer, error) {
	var config pipelineTracerConfig
	if cfg != nil {
		if err := json.Unmarshal(cfg, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config: %v", err)
		}
	}
	config.fillDefaultValues()

	log.Info("NewPipelineTracer", "config", config)

	t := &PipelineTracer{
		config: config,
	}
	return t, nil
}

func (t *PipelineTracer) OnBlockchainInit(chainConfig *params.ChainConfig) {
	log.Info("Init pipeline with param", "chainConfig", chainConfig.ChainId.String(), "config", t.config)

	// set default leader key: <chain_id>/[<version>/]writers/leader
	if t.config.LeaderKey == "" {
		t.config.LeaderKey = failover.BuildLeaderKey(chainConfig.ChainId.String(), t.config.Version)
	}

	if t.config.Topic == "" {
		if t.config.Version == "" {
			t.config.Topic = fmt.Sprintf("nodex_pipeline_%d", chainConfig.ChainId)
		} else {
			t.config.Topic = fmt.Sprintf("nodex_pipeline_%d_%s", chainConfig.ChainId, t.config.Version)
		}
	}

	// Initialize pipeline
	err := InitPipeline(t.config.Region, t.config.NodeXBucket, t.config.ChainTableBucket,
		t.config.Brokers, t.config.Topic, chainConfig.ChainId.String(), t.config.Version,
		t.config.S3TempDir)

	if err != nil {
		log.Crit("Failed to init pipeline", "err", err)
	}

	// Setup writer failover. 主备切换完全在 pipeline 内完成，geth 不感知。
	err = SetupFailover(failover.Config{
		EtcdEndpoints: t.config.EtcdEndpoints,
		LeaderKey:     t.config.LeaderKey,
		NodeID:        t.config.NodeID,
		IsBackup:      t.config.IsBackup,
		LockTTL:       t.config.LockTTL,
		CheckInterval: time.Duration(t.config.CheckInterval) * time.Second,
	})
	if err != nil {
		log.Crit("Failed to setup failover", "err", err)
	}

	// start upload work should be after leader election
	err = NodeXPusher.StartUploadWork()
	if err != nil {
		log.Crit("Failed to start NodeXPusher upload work", "err", err)
	}
	err = ChainTableBucketPusher.StartUploadWork()
	if err != nil {
		log.Crit("Failed to start ChainTableBucketPusher upload work", "err", err)
	}

	metrics.NodeInfo.Update(map[string]string{
		"chain_id": chainConfig.ChainId.String(),
		"role":     "writer",
	})
}

func (t *PipelineTracer) OnClose() {
	// Close the failover manager. 运维指派的 leader key 不删除，
	// 它跨进程重启保持权威；只释放本节点持有的 writeLock。
	if manager := failover.GlobalManager; manager != nil {
		if err := manager.Close(); err != nil {
			log.Error("Failed to close failover manager", "err", err)
		}
		failover.GlobalManager = nil
	}
	// Close processors
	if NodeXPusher != nil {
		NodeXPusher.Close()
	}
	if ChainTableBucketPusher != nil {
		ChainTableBucketPusher.Close()
	}
}

func (t *PipelineTracer) OnBlockStart(block *types.Block) {
	BlockCtx = &ExtraInfo{
		BlockNumber: block.Number().Uint64(),
		BlockHash:   block.Hash(),
	}
	BlockCtx.BlockDiff = &ptypes.BlockStorageDiff{}
	BlockCtx.BlockHeader = util.BuildPilelineBlockHeader(block)
	BlockCtx.BlockFile = &ptypes.BlockFile{
		Block:            util.BuildPipelineBlock(block),
		Events:           make([]ptypes.Event, 0),
		Txs:              make([]ptypes.Transaction, 0),
		Traces:           make([]ptypes.Trace, 0),
		ErrorEvents:      make([]ptypes.Event, 0),
		ErrorTraces:      make([]ptypes.Trace, 0),
		StorageContracts: make([]string, 0),
	}
	BlockCtx.Tx = nil
	BlockCtx.From = common.Address{}
	BlockCtx.BlockStartTime = time.Now()
	BlockCtx.Committed = false
	BlockCtx.ChangeContracts = make(map[common.Address]struct{})
}

func (t *PipelineTracer) OnBlockEnd(blockErr error) {
	// empty block process
	if !BlockCtx.Committed {
		t.OnCommit(BlockCtx.BlockHeader.StateRoot, BlockCtx.BlockHeader.StateRoot, nil, nil, nil, nil, nil, nil)
	}

	// push block change notification
	if BlockCtx.BlockChange != nil {
		start := time.Now()
		err := NodeXPusher.PushBlockChangeNotification(BlockCtx.BlockChange, nil)
		if err == nil {
			log.Info("Push kafka", "dropBlocks", BlockCtx.BlockChange.DropBlocks, "newBlocks", BlockCtx.BlockChange.NewBlocks, "kafka elapsed", common.PrettyDuration(time.Since(start)))
		} else {
			log.Error("Failed to push kafka", "err", err, "dropBlocks", BlockCtx.BlockChange.DropBlocks, "newBlocks", BlockCtx.BlockChange.NewBlocks)
		}
	}
	metrics.BlockProcessTimer.UpdateSince(BlockCtx.BlockStartTime)

	// TODO on commit
}

func (t *PipelineTracer) OnTxStart(tx *types.Transaction, from common.Address) {
	callTracer := newCallTracerRaw(BlockCtx.ChangeContracts, BlockCtx.BlockFile)
	t.callTracer = callTracer
	t.callTracer.OnTxStart(tx, from)

	BlockCtx.Tx = tx
	BlockCtx.From = from
	BlockCtx.TxStartTime = time.Now()
}

func (t *PipelineTracer) OnTxEnd(receipt *types.Receipt, err error) {
	defer func() {
		metrics.BlockTxExecutionTimer.UpdateSince(BlockCtx.TxStartTime)
	}()
	t.callTracer.OnTxEnd(receipt, err)
	t.callTracer = nil
	tx := util.BuildPipelineTransaction(BlockCtx.Tx, receipt, BlockCtx.From, BlockCtx.BlockHeader.BaseFeePerGas.ToInt())
	BlockCtx.BlockFile.Txs = append(BlockCtx.BlockFile.Txs, tx)
}

func (t *PipelineTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	if t.callTracer != nil {
		t.callTracer.CaptureStart(env, from, to, create, input, gas, value)
	}
}

func (t *PipelineTracer) CaptureEnd(output []byte, gasUsed uint64, ti time.Duration, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureEnd(output, gasUsed, ti, err)
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

func (t *PipelineTracer) CaptureState(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureState(env, pc, op, gas, cost, scope, rData, depth, err)
	}
}

func (t *PipelineTracer) CaptureFault(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureFault(env, pc, op, gas, cost, scope, depth, err)
	}
}

func (t *PipelineTracer) CaptureTxStart(gas uint64) {
}

func (t *PipelineTracer) CaptureTxEnd(restGas uint64) {
}

func (t *PipelineTracer) OnLog(log *types.Log) {
	if t.callTracer != nil {
		t.callTracer.OnLog(log)
	}
}

func (t *PipelineTracer) OnGenesisBlock(block *types.Block, alloc ptypes.GenesisAlloc) {
	if NodeXPusher.LastPushedBlock() != nil {
		return
	}

	// 内部s3
	header := util.BuildPilelineBlockHeader(block)
	err := uploadBlockHeader(header)
	if err != nil {
		log.Crit("Failed to upload block", "err", err)
	}
	log.Info("[inner s3] 1.upload genesis block", "block hash", block.Hash().Hex(), "block number", block.Number().Uint64())

	blockDiff := GenesisAllocToStateDiff(alloc)
	blockDiff.Hash = block.Root()
	// genesis block has no parent
	blockDiff.ParentHash = types.EmptyRootHash
	err = uploadBlockDiff(blockDiff)
	if err != nil {
		log.Crit("Failed to upload block diff files to s3", "err", err)
	}
	log.Info("[inner s3] 2.upload genesis state diff", "block", block.Hash().Hex())

	blockFile := BuildGenesisBlockFile(block, alloc)

	// upload block file and meta data
	err = uploadBlockFile(blockFile)
	if err != nil {
		log.Crit("Failed to upload block files to s3", "err", err)
	}
	log.Info("3.upload block file", "block hash", header.Hash.Hex(), "block number", header.Number.ToInt().Uint64(),
		"txs", len(blockFile.Txs), "traces", len(blockFile.Traces))

	// upload block file validation
	err = uploadblockFileValidation(blockFile)
	if err != nil {
		log.Crit("Failed to upload file validation to s3", "err", err)
	}
	log.Info("4.upload block file validation", "block hash", header.Hash.Hex(), "block number", header.Number.ToInt().Uint64())

	// push block change notification
	blockChanges := &ptypes.BlockChangeNotification{
		ChangeType: 1,
		NewBlocks: []ptypes.BlockContext{
			{
				Hash:        block.Hash(),
				ParentHash:  block.ParentHash(),
				BlockNumber: block.NumberU64(),
				Timestamp:   block.Time().Uint64(),
			},
		},
	}

	err = NodeXPusher.PushBlockChangeNotification(blockChanges, nil)
	if err != nil {
		log.Crit("Failed to push block change notification", "err", err)
	}

	log.Info("push genesis block change notification", "block hash", block.Hash().Hex(), "block number", block.Number().Uint64())
}

func (t *PipelineTracer) OnCommit(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, accountsOrigin map[common.Address][]byte, storages map[common.Hash]map[common.Hash][]byte, storagesOrigin map[common.Address]map[common.Hash][]byte, codes map[common.Hash][]byte) {
	if originRoot != root {
		var stateDiff *ptypes.BlockStorageDiff
		stateDiff = stateUpdateToStateDiff(originRoot, root, destructs, accounts, accountsOrigin, storages, storagesOrigin, codes)
		BlockCtx.BlockDiff = stateDiff
	} else {
		BlockCtx.BlockDiff = nil
	}

	for addr := range BlockCtx.ChangeContracts {
		BlockCtx.BlockFile.StorageContracts = append(BlockCtx.BlockFile.StorageContracts, strings.ToLower(util.AddressToHex(addr)))
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var uploadErrs []error

	// Helper function to handle errors safely
	handleError := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			uploadErrs = append(uploadErrs, err)
		}
	}

	// 上传 block head
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := uploadBlockHeader(BlockCtx.BlockHeader)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 上传 state diff
	wg.Add(1)
	go func() {
		defer wg.Done()
		if BlockCtx.BlockDiff == nil {
			return
		}
		err := uploadBlockDiff(BlockCtx.BlockDiff)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 上传 block file
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := uploadBlockFile(BlockCtx.BlockFile)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 上传 block file validation
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := uploadblockFileValidation(BlockCtx.BlockFile)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 等待所有上传完成
	wg.Wait()

	log.Info("Upload block", "block number", BlockCtx.BlockNumber, "block hash", BlockCtx.BlockHash.Hex())

	// 检查是否有错误
	if len(uploadErrs) > 0 {
		for _, err := range uploadErrs {
			log.Error("Upload error", "err", err)
		}
		log.Crit("One or more uploads failed")
	}

	BlockCtx.Committed = true

	metrics.LatestUploadedBlockNumber.Update(int64(BlockCtx.BlockNumber))
}
