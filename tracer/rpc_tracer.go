package tracer

import (
	"encoding/json"
	"math/big"
	"strings"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
)

var _ vm.EVMLogger = (*RPCTracer)(nil)

// RPCTracer is a lightweight tracer for trace_debankBlock RPC.
// It wraps callTracer to collect traces/events in-memory without S3/Kafka uploads.
type RPCTracer struct {
	callTracer   *callTracer
	currentBlock *RPCBlockContext
}

// RPCBlockContext holds per-block state for the RPC tracer.
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

// OnTxStart initializes a new callTracer for the transaction.
func (t *RPCTracer) OnTxStart(tx *types.Transaction, from common.Address) {
	ct := newCallTracerRaw(t.currentBlock.ChangeContracts, t.currentBlock.BlockFile)
	t.callTracer = ct
	t.callTracer.OnTxStart(tx, from)
	t.currentBlock.Tx = tx
	t.currentBlock.From = from
}

// SetTxHash overrides the tx hash used for trace IDs and tx.ID.
// Used by IoTeX to set the native action hash instead of geth RLP hash.
func (t *RPCTracer) SetTxHash(hash string) {
	if t.callTracer != nil {
		t.callTracer.txID = hash
	}
	t.currentBlock.TxHashOverride = hash
}

// OnTxEnd finalizes the transaction trace and builds the pipeline transaction.
func (t *RPCTracer) OnTxEnd(receipt *types.Receipt, err error) {
	if t.callTracer == nil {
		return
	}
	t.callTracer.OnTxEnd(receipt, err)
	t.callTracer = nil

	tx := util.BuildPipelineTransaction(t.currentBlock.Tx, receipt, t.currentBlock.From, t.currentBlock.BlockHeader.BaseFeePerGas.ToInt())
	if t.currentBlock.TxHashOverride != "" {
		tx.ID = t.currentBlock.TxHashOverride
		t.currentBlock.TxHashOverride = ""
	}
	t.currentBlock.BlockFile.Txs = append(t.currentBlock.BlockFile.Txs, tx)
}

// vm.EVMLogger interface — delegate to callTracer

func (t *RPCTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	if t.callTracer != nil {
		t.callTracer.CaptureStart(env, from, to, create, input, gas, value)
	}
}

func (t *RPCTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureEnd(output, gasUsed, err)
	}
}

func (t *RPCTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.callTracer != nil {
		t.callTracer.CaptureEnter(typ, from, to, input, gas, value)
	}
}

func (t *RPCTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureExit(output, gasUsed, err)
	}
}

func (t *RPCTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureState(pc, op, gas, cost, scope, rData, depth, err)
	}
}

func (t *RPCTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.CaptureFault(pc, op, gas, cost, scope, depth, err)
	}
}

func (t *RPCTracer) CaptureTxStart(gasLimit uint64) {
}

func (t *RPCTracer) CaptureTxEnd(restGas uint64) {
}

func (t *RPCTracer) OnLog(l *types.Log) {
	if t.callTracer != nil {
		t.callTracer.OnLog(l)
	}
}

// InsertLog forwards to callTracer.InsertLog. See callTracer.InsertLog for the
// caller contract (callstack must still contain the root frame, traceAddress
// must point to a frame whose CaptureExit has already finalized it into its
// parent.Calls, position must be the OnLog-time snapshot).
func (t *RPCTracer) InsertLog(traceAddress []int64, position int64, l *types.Log) {
	if t.callTracer != nil {
		t.callTracer.InsertLog(traceAddress, position, l)
	}
}

// GetOutPut builds the final DebankOutPut from collected traces and state diff.
func (t *RPCTracer) GetOutPut(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storages map[common.Hash]map[common.Hash][]byte, codes map[common.Hash][]byte) *ptypes.DebankOutPut {
	if originRoot != root {
		t.currentBlock.BlockDiff = stateUpdateToStateDiff(originRoot, root, destructs, accounts, nil, storages, nil, codes)
	} else {
		t.currentBlock.BlockDiff = nil
	}

	for addr := range t.currentBlock.ChangeContracts {
		t.currentBlock.BlockFile.StorageContracts = append(t.currentBlock.BlockFile.StorageContracts, strings.ToLower(addr.Hex()))
	}

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
