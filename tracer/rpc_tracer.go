package tracer

import (
	"encoding/json"
	"math/big"
	"strings"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

type RPCTracer struct {
	callTracer   *callTracer
	currentBlock *RPCBlockContext
}

type RPCBlockContext struct {
	BlockDiff       *ptypes.BlockStorageDiff
	BlockHeader     *ptypes.Header
	BlockFile       *ptypes.BlockFile
	Tx              *types.Transaction
	From            common.Address
	ChangeContracts map[common.Address]struct{}
	TxHashOverride  string // if set, overrides tx.ID in OnTxEnd
}

func (t *RPCTracer) Stop(err error) {
	return
}

func (t *RPCTracer) GetResult() (json.RawMessage, error) {
	return nil, nil
}

func (t *RPCTracer) OnBlockStart(block *types.Block) {
	t.currentBlock = &RPCBlockContext{
		ChangeContracts: make(map[common.Address]struct{}),
	}
	t.currentBlock.BlockDiff = &ptypes.BlockStorageDiff{}
	t.currentBlock.BlockHeader = util.BuildPilelineBlockHeader(block)
	t.currentBlock.BlockFile = &ptypes.BlockFile{
		Block:            util.BuildPipelineBlock(block),
		Events:           make([]ptypes.Event, 0),
		Txs:              make([]ptypes.Transaction, 0),
		Traces:           make([]ptypes.Trace, 0),
		ErrorEvents:      make([]ptypes.Event, 0),
		ErrorTraces:      make([]ptypes.Trace, 0),
		StorageContracts: make([]string, 0),
	}
}

func (t *RPCTracer) OnEnter(depth int, typ byte, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.callTracer != nil {
		t.callTracer.OnEnter(depth, typ, from, to, input, gas, value)
	}
}

func (t *RPCTracer) OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if t.callTracer != nil {
		t.callTracer.OnExit(depth, output, gasUsed, err, reverted)
	}
}

// SetTxHash overrides the tx hash used for trace IDs and tx.ID.
// Used by IoTeX to set the native action hash instead of geth RLP hash.
func (t *RPCTracer) SetTxHash(hash string) {
	if t.callTracer != nil {
		t.callTracer.txID = hash
	}
	t.currentBlock.TxHashOverride = hash
}

func (t *RPCTracer) OnOpcode(pc uint64, opcode byte, gas, cost uint64, scope tracing.OpContext, rData []byte, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.OnOpcode(pc, opcode, gas, cost, scope, rData, depth, err)
	}
}

func (t *RPCTracer) OnTxStart(env *tracing.VMContext, tx *types.Transaction, from common.Address) {
	callTracer := newCallTracerRaw(t.currentBlock.ChangeContracts, t.currentBlock.BlockFile)
	t.callTracer = callTracer
	t.callTracer.OnTxStart(env, tx, from)
	t.currentBlock.From = from
	t.currentBlock.Tx = tx
}

func (t *RPCTracer) OnTxEnd(receipt *types.Receipt, err error) {
	t.callTracer.OnTxEnd(receipt, err)
	t.callTracer = nil

	tx := util.BuildPipelineTransaction(t.currentBlock.Tx, receipt, t.currentBlock.From, t.currentBlock.BlockHeader.BaseFeePerGas.ToInt())
	if t.currentBlock.TxHashOverride != "" {
		tx.ID = t.currentBlock.TxHashOverride
		t.currentBlock.TxHashOverride = ""
	}
	t.currentBlock.BlockFile.Txs = append(t.currentBlock.BlockFile.Txs, tx)
}

func (t *RPCTracer) OnLog(log *types.Log) {
	if t.callTracer != nil {
		t.callTracer.OnLog(log)
	}
}

func (t *RPCTracer) GetOutPut(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storages map[common.Hash]map[common.Hash][]byte, codes map[common.Hash][]byte) *ptypes.DebankOutPut {
	if originRoot != root {
		t.currentBlock.BlockDiff = stateUpdateToStateDiff(originRoot, root, destructs, accounts, nil, storages, nil, codes)
	} else {
		t.currentBlock.BlockDiff = nil
	}

	for addr := range t.currentBlock.ChangeContracts {
		t.currentBlock.BlockFile.StorageContracts = append(t.currentBlock.BlockFile.StorageContracts, strings.ToLower(addr.Hex()))
	}

	// Generate DebankOutPut
	var stateDiffBytes []byte
	var err error
	if t.currentBlock.BlockDiff != nil {
		stateDiffBytes, err = util.EncodeToRlp(t.currentBlock.BlockDiff)
		if err != nil {
			log.Error("Failed to encode state diff", "err", err)
			stateDiffBytes = []byte{}
		}
	} else {
		stateDiffBytes = []byte{}
	}

	return &ptypes.DebankOutPut{
		BlockFile:      t.currentBlock.BlockFile,
		Header:         t.currentBlock.BlockHeader,
		StateDiff:      hexutil.Bytes(stateDiffBytes),
		ValidationHash: t.currentBlock.BlockFile.Validation().ValidationHash,
	}
}
