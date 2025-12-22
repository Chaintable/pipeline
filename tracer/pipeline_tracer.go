package tracer

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/kaiachain/kaia/crypto"

	"github.com/Chaintable/pipeline/metrics"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/blockchain/vm"
	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/params"
)

// 需要上传3种data
// 1. block
// 2. state diff
// 3. block file

var _ EVMLogger = (*PipelineTracer)(nil)

type PipelineTracer struct {
	config     PipelineTracerConfig
	callTracer *callTracer
}

type PipelineTracerConfig struct {
	Region           string   `json:"region"`
	NodeXBucket      string   `json:"node_x_bucket"`
	ChainTableBucket string   `json:"chain_table_bucket"`
	Brokers          []string `json:"brokers"`
	Topic            string   `json:"topic"`
	S3TempDir        string   `json:"s3_temp_dir"`
	IsBackup         bool     `json:"is_backup"`
}

func NewPipelineTracer(cfg json.RawMessage) (*PipelineTracer, error) {
	var config PipelineTracerConfig
	if cfg != nil {
		if err := json.Unmarshal(cfg, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config: %v", err)
		}
	}
	t := &PipelineTracer{
		config: config,
	}

	return t, nil
}

func (t *PipelineTracer) OnBlockchainInit(chainConfig *params.ChainConfig) {
	log.Println("Init pipeline with param", "chainConfig", chainConfig.ChainID.String(), "config", t.config)
	err := InitPipeline(t.config.Region, t.config.NodeXBucket, t.config.ChainTableBucket, t.config.Brokers, t.config.Topic, chainConfig.ChainID.String(), t.config.S3TempDir, t.config.IsBackup)
	if err != nil {
		log.Panicln("Failed to init pipeline", "err", err)
	}
}

func (t *PipelineTracer) OnClose() {
	NodeXPusher.Close()
}

func (t *PipelineTracer) OnBlockStart(block *types.Block) {
	BlockCtx = &ExtraInfo{
		BlockNumber: block.NumberU64(),
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
		err := NodeXPusher.PushBlockChangeNotification(BlockCtx.BlockChange)
		if err == nil {
			log.Println("Push kafka", "dropBlocks", BlockCtx.BlockChange.DropBlocks, "newBlocks", BlockCtx.BlockChange.NewBlocks, "kafka elapsed", common.PrettyDuration(time.Since(start)))
		} else {
			log.Println("Failed to push kafka", "err", err, "dropBlocks", BlockCtx.BlockChange.DropBlocks, "newBlocks", BlockCtx.BlockChange.NewBlocks)
		}
	}
	metrics.BlockProcessTimer.UpdateSince(BlockCtx.BlockStartTime)
}

func (t *PipelineTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	if t.callTracer != nil {
		t.callTracer.CaptureStart(env, from, to, create, input, gas, value)
	}
}

func (t *PipelineTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureEnd(output, gasUsed, err)
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

func (t *PipelineTracer) CaptureFault(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost, ccLeft, ccOpcode uint64, scope *vm.ScopeContext, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureFault(env, pc, op, gas, cost, ccLeft, ccOpcode, scope, depth, err)
	}
}

func (t *PipelineTracer) CaptureTxStart(gas uint64) {
}

func (t *PipelineTracer) CaptureTxEnd(restGas uint64) {
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

func (t *PipelineTracer) CaptureState(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost, ccLeft, ccOpcode uint64, scope *vm.ScopeContext, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureState(env, pc, op, gas, cost, ccLeft, ccOpcode, scope, depth, err)
	}
}

func (t *PipelineTracer) OnLog(log *types.Log) {
	if t.callTracer != nil {
		t.callTracer.OnLog(log)
	}
}

func (t *PipelineTracer) OnGenesisBlock(block *types.Block, alloc ptypes.GenesisAlloc) {
	if NodeXPusher.LastBlockNotice != nil {
		return
	}

	// 内部s3
	header := util.BuildPilelineBlockHeader(block)
	err := uploadBlockHeader(header)
	if err != nil {
		log.Panicln("Failed to upload block", "err", err)
	}
	log.Println("[inner s3] 1.upload genesis block", "block hash", block.Hash().Hex(), "block number", block.Number().Uint64())

	blockDiff := GenesisAllocToStateDiff(alloc)
	blockDiff.Hash = block.Root()
	// genesis block has no parent
	blockDiff.ParentHash = types.EmptyRootHash
	err = uploadBlockDiff(blockDiff)
	if err != nil {
		log.Panicln("Failed to upload block diff files to s3", "err", err)
	}
	log.Println("[inner s3] 2.upload genesis state diff", "block", block.Hash().Hex())

	// 业务s3
	blockFile := &ptypes.BlockFile{
		Block:            util.BuildPipelineBlock(block),
		Txs:              make([]ptypes.Transaction, 0),
		Events:           make([]ptypes.Event, 0),
		Traces:           make([]ptypes.Trace, 0),
		ErrorEvents:      make([]ptypes.Event, 0),
		ErrorTraces:      make([]ptypes.Trace, 0),
		StorageContracts: make([]string, 0),
	}
	for addr, account := range alloc {
		if len(account.Storage) > 0 {
			blockFile.StorageContracts = append(blockFile.StorageContracts, strings.ToLower(addr.Hex()))
		}
	}
	// upload block file and meta data
	err = uploadBlockFile(blockFile)
	if err != nil {
		log.Panicln("Failed to upload block files to s3", "err", err)
	}
	log.Println("3.upload block file", "block hash", header.Hash.Hex(), "block number", header.Number.ToInt().Uint64())

	// upload block file validation
	err = uploadblockFileValidation(blockFile)
	if err != nil {
		log.Panicln("Failed to upload file validation to s3", "err", err)
	}
	log.Println("4.upload block file validation", "block hash", header.Hash.Hex(), "block number", header.Number.ToInt().Uint64())

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

	err = NodeXPusher.PushBlockChangeNotification(blockChanges)
	if err != nil {
		log.Panicln("Failed to push block change notification", "err", err)
	}

	log.Println("push genesis block change notification", "block hash", block.Hash().Hex(), "block number", block.Number().Uint64())
}

func (t *PipelineTracer) OnCommit(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, accountsOrigin map[common.Address][]byte, storages map[common.Hash]map[common.Hash][]byte, storagesOrigin map[common.Address]map[common.Hash][]byte, codes map[common.Hash][]byte) {
	if originRoot != root {
		BlockCtx.BlockDiff = stateUpdateToStateDiff(originRoot, root, destructs, accounts, accountsOrigin, storages, storagesOrigin, codes)
	} else {
		BlockCtx.BlockDiff = nil
	}

	for addr := range BlockCtx.ChangeContracts {
		BlockCtx.BlockFile.StorageContracts = append(BlockCtx.BlockFile.StorageContracts, strings.ToLower(addr.Hex()))
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

	if t.config.IsBackup {
		log.Println("Backup Upload block", "block number", BlockCtx.BlockNumber, "block hash", BlockCtx.BlockHash.Hex())
	} else {
		log.Println("Upload block", "block number", BlockCtx.BlockNumber, "block hash", BlockCtx.BlockHash.Hex())
	}

	// 检查是否有错误
	if len(uploadErrs) > 0 {
		for _, err := range uploadErrs {
			log.Println("Upload error", "err", err)
		}
		log.Println("One or more uploads failed")
	}

	BlockCtx.Committed = true

	metrics.LatestUploadedBlockNumber.Update(int64(BlockCtx.BlockNumber))
}

func addressToHash(a common.Address) common.Hash {
	return crypto.Keccak256Hash(a.Bytes())
}
