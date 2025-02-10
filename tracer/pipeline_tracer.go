package tracer

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/Chaintable/pipeline/metrics"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// 需要上传3种data
// 1. block
// 2. state diff
// 3. block file

type PipelineTracer struct {
	config     pipelineTracerConfig
	callTracer *callTracer
}

type pipelineTracerConfig struct {
	Region           string   `json:"region"`
	NodeXBucket      string   `json:"node_x_bucket"`
	ChainTableBucket string   `json:"chain_table_bucket"`
	Brokers          []string `json:"brokers"`
	Topic            string   `json:"topic"`
	S3TempDir        string   `json:"s3_temp_dir"`
}

func NewPipelineTracer(cfg json.RawMessage) (*PipelineTracer, error) {
	var config pipelineTracerConfig
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
	log.Info("Init pipeline with param", "chainConfig", chainConfig.ChainID.String(), "config", t.config)
	err := InitPipeline(t.config.Region, t.config.NodeXBucket, t.config.ChainTableBucket, t.config.Brokers, t.config.Topic, chainConfig.ChainID.String(), t.config.S3TempDir)
	if err != nil {
		log.Crit("Failed to init pipeline", "err", err)
	}
	metrics.NodeInfo.Update(map[string]string{
		"chain_id": chainConfig.ChainID.String(),
		"role":     "writer",
	})
}

func (t *PipelineTracer) OnClose() {
	NodeXPusher.Close()
}

func (t *PipelineTracer) OnBlockStart(event tracing.BlockEvent) {
	BlockCtx = &ExtraInfo{
		BlockNumber: event.Block.Number().Uint64(),
		BlockHash:   event.Block.Hash(),
	}
	BlockCtx.BlockDiff = &ptypes.BlockStorageDiff{}
	BlockCtx.BlockHeader = util.BuildPilelineBlockHeader(event.Block)
	BlockCtx.RawBlock = event.Block
	BlockCtx.BlockFile = &ptypes.BlockFile{
		Block:            util.BuildPipelineBlock(event.Block),
		SpecialTransfers: util.BuildPipelineWithdrawals(event.Block),
		Events:           make([]ptypes.Event, 0),
		Txs:              make([]ptypes.Transaction, 0),
		Traces:           make([]ptypes.Trace, 0),
	}
	BlockCtx.Tx = nil
	BlockCtx.From = common.Address{}
	BlockCtx.BlockStartTime = time.Now()
	BlockCtx.Committed = false
	BlockCtx.AccountLoads = make(map[common.Address]struct{})
	BlockCtx.StorageLoads = make(map[common.Address]map[common.Hash]struct{})
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
			log.Info("Push kafka", "dropBlocks", BlockCtx.BlockChange.DropBlocks, "newBlocks", BlockCtx.BlockChange.NewBlocks, "kafka elapsed", common.PrettyDuration(time.Since(start)))
		} else {
			log.Error("Failed to push kafka", "err", err, "dropBlocks", BlockCtx.BlockChange.DropBlocks, "newBlocks", BlockCtx.BlockChange.NewBlocks)
		}
	}
	metrics.BlockProcessTimer.UpdateSince(BlockCtx.BlockStartTime)
}

func (t *PipelineTracer) OnTxStart(vm *tracing.VMContext, tx *types.Transaction, from common.Address) {
	callTracer := newCallTracerRaw()
	t.callTracer = callTracer
	t.callTracer.OnTxStart(vm, tx, from)
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

	tx := util.BuildPipelineTransaction(BlockCtx.Tx, receipt, BlockCtx.From)
	BlockCtx.BlockFile.Txs = append(BlockCtx.BlockFile.Txs, tx)
}

func (t *PipelineTracer) OnEnter(depth int, typ byte, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.callTracer == nil {
		return
	}
	t.callTracer.OnEnter(depth, typ, from, to, input, gas, value)
}

func (t *PipelineTracer) OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if t.callTracer == nil {
		return
	}
	t.callTracer.OnExit(depth, output, gasUsed, err, reverted)
}

func (t *PipelineTracer) OnOpcode(pc uint64, op byte, gas, cost uint64, scope tracing.OpContext, rData []byte, depth int, err error) {
	if t.callTracer == nil {
		return
	}
	t.callTracer.OnOpcode(pc, op, gas, cost, scope, rData, depth, err)
}

func (t *PipelineTracer) OnLog(log *types.Log) {
	if t.callTracer == nil {
		return
	}
	t.callTracer.OnLog(log)
}

func (t *PipelineTracer) OnBalanceChange(a common.Address, prevBalance, newBalance *big.Int, reason tracing.BalanceChangeReason) {
	diff := new(big.Int).Sub(newBalance, prevBalance)

	if reason == tracing.BalanceIncreaseRewardMineUncle || reason == tracing.BalanceIncreaseRewardMineBlock {
		for i := range BlockCtx.BlockFile.SpecialTransfers {
			sp := &BlockCtx.BlockFile.SpecialTransfers[i]
			if sp.ToAddress == strings.ToLower(a.Hex()) && sp.Memo == "block_reward" {
				sp.Value = (*hexutil.Big)(new(big.Int).Add(sp.Value.ToInt(), diff))
				return
			}
		}
		specialTransfer := ptypes.SpecialTransfer{
			FromAddress: common.Address{}.Hex(),
			ToAddress:   strings.ToLower(a.Hex()),
			Value:       (*hexutil.Big)(diff),
			Memo:        "block_reward",
			Idx:         big.NewInt(int64(reason)),
		}
		specialTransfer.ID = util.ToHash([]string{BlockCtx.BlockHash.Hex(), specialTransfer.ToAddress, fmt.Sprintf("%d", tracing.BalanceIncreaseRewardMineBlock)})
		BlockCtx.BlockFile.SpecialTransfers = append(BlockCtx.BlockFile.SpecialTransfers, specialTransfer)
	}
	if reason == tracing.BalanceIncreaseRewardTransactionFee {
		for i := range BlockCtx.BlockFile.SpecialTransfers {
			sp := &BlockCtx.BlockFile.SpecialTransfers[i]
			if sp.ToAddress == strings.ToLower(a.Hex()) && sp.Memo == "gasfee_reward" {
				sp.Value = (*hexutil.Big)(new(big.Int).Add(sp.Value.ToInt(), diff))
				return
			}
		}
		specialTransfer := ptypes.SpecialTransfer{
			FromAddress: common.Address{}.Hex(),
			ToAddress:   strings.ToLower(a.Hex()),
			Value:       (*hexutil.Big)(diff),
			Memo:        "gasfee_reward",
			Idx:         big.NewInt(int64(reason)),
		}
		specialTransfer.ID = util.ToHash([]string{BlockCtx.BlockHash.Hex(), specialTransfer.ToAddress, fmt.Sprintf("%d", tracing.BalanceIncreaseRewardTransactionFee)})
		BlockCtx.BlockFile.SpecialTransfers = append(BlockCtx.BlockFile.SpecialTransfers, specialTransfer)
	}
}

func (t *PipelineTracer) OnGenesisBlock(block *types.Block, alloc types.GenesisAlloc) {
	if NodeXPusher.LastBlockNotice != nil {
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

	// 业务s3
	blockFile := &ptypes.BlockFile{
		Block: util.BuildPipelineBlock(block),
	}
	for addr, acc := range alloc {
		if acc.Balance.Cmp(big.NewInt(0)) > 0 {
			specialTransfer := ptypes.SpecialTransfer{
				FromAddress: common.Address{}.Hex(),
				ToAddress:   strings.ToLower(addr.Hex()),
				Value:       (*hexutil.Big)(acc.Balance),
				Memo:        "genesis",
				Idx:         big.NewInt(0),
			}
			specialTransfer.ID = util.ToHash([]string{block.Hash().Hex(), specialTransfer.ToAddress, fmt.Sprintf("%d", specialTransfer.Idx)})
			blockFile.SpecialTransfers = append(blockFile.SpecialTransfers, specialTransfer)
		}
	}
	// upload block file and meta data
	err = uploadBlockFile(blockFile)
	if err != nil {
		log.Crit("Failed to upload block files to s3", "err", err)
	}
	log.Info("3.upload block file", "block hash", header.Hash.Hex(), "block number", header.Number.ToInt().Uint64())

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
				Timestamp:   block.Time(),
			},
		},
	}

	err = NodeXPusher.PushBlockChangeNotification(blockChanges)
	if err != nil {
		log.Crit("Failed to push block change notification", "err", err)
	}

	log.Info("push genesis block change notification", "block hash", block.Hash().Hex(), "block number", block.Number().Uint64())
}

func (t *PipelineTracer) OnCommit(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, accountsOrigin map[common.Address][]byte, storages map[common.Hash]map[common.Hash][]byte, storagesOrigin map[common.Address]map[common.Hash][]byte, codes map[common.Hash][]byte) {
	if originRoot != root {
		stateDiff := stateUpdateToStateDiff(originRoot, root, destructs, accounts, accountsOrigin, storages, storagesOrigin, codes)
		BlockCtx.BlockDiff = stateDiff
	} else {
		BlockCtx.BlockDiff = nil
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

	wg.Add(1)
	go func() {
		defer wg.Done()

		if len(BlockCtx.AccountLoads) == 0 && len(BlockCtx.StorageLoads) == 0 {
			return
		}
		blockStateLoad := &ptypes.BlockLoad{
			Hash: BlockCtx.RawBlock.Hash(),
		}
		for addr := range BlockCtx.AccountLoads {
			blockStateLoad.AccountLoads = append(blockStateLoad.AccountLoads, addr)
		}
		for addr, keys := range BlockCtx.StorageLoads {
			stateLoad := &ptypes.AccountStorageLoad{
				Address: addr,
			}
			for key := range keys {
				stateLoad.Keys = append(stateLoad.Keys, key)
			}
			blockStateLoad.StorageLoads = append(blockStateLoad.StorageLoads, *stateLoad)
		}
		err := uploadBlockStateLoad(blockStateLoad)
		if err != nil {
			handleError(err)
			return
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := uploadRawBlock(BlockCtx.RawBlock)
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

func (t *PipelineTracer) OnAccountRead(addr common.Address) {
	BlockCtx.AccountLoads[addr] = struct{}{}
}

func (t *PipelineTracer) OnStorageRead(addr common.Address, key common.Hash) {
	if _, ok := BlockCtx.StorageLoads[addr]; !ok {
		BlockCtx.StorageLoads[addr] = make(map[common.Hash]struct{})
	}
	BlockCtx.StorageLoads[addr][key] = struct{}{}
}
