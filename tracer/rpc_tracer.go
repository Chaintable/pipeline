package tracer

import (
	"fmt"
	"strings"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/common/hexutil"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/core/vm"
	"github.com/ethereum-optimism/optimism/l2geth/log"
)

type RPCTracer struct {
	callTracer    *callTracer
	currentBlock  *RPCBlockContext
	currentTx     *types.Transaction
	currentTxFrom common.Address
}

type RPCBlockContext struct {
	BlockDiff       *ptypes.BlockStorageDiff
	BlockHeader     *ptypes.Header
	BlockFile       *ptypes.BlockFile
	ChangeContracts map[common.Address]struct{}
}

func NewRPCTracer() *RPCTracer {
	return &RPCTracer{}
}

func (t *RPCTracer) OnBlockStart(block *types.Block) {
	t.currentBlock = &RPCBlockContext{
		ChangeContracts: make(map[common.Address]struct{}),
	}
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

func (t *RPCTracer) OnTxStart(tx *types.Transaction, from common.Address) {
	t.callTracer = newCallTracerRaw(t.currentBlock.ChangeContracts, t.currentBlock.BlockFile)
	t.callTracer.OnTxStart(tx, from)
	t.currentTx = tx
	t.currentTxFrom = from
}

func (t *RPCTracer) GetCallTracer() vm.EVMLogger {
	return t.callTracer
}

func (t *RPCTracer) OnTxEnd(receipt *types.Receipt, err error) error {
	if err != nil {
		return err
	}
	if receipt == nil {
		return fmt.Errorf("missing transaction receipt")
	}
	if t.callTracer == nil || len(t.callTracer.callstack) != 1 {
		return fmt.Errorf("missing or unbalanced transaction call trace")
	}
	// A matching state root alone cannot detect an omitted failed root call.
	if len(receipt.PostState) == 0 && t.callTracer.callstack[0].failed() != (receipt.Status == types.ReceiptStatusFailed) {
		return fmt.Errorf("root call trace status does not match receipt")
	}
	t.callTracer.OnTxEnd(receipt, err)

	baseFee := t.currentBlock.BlockHeader.BaseFeePerGas.ToInt()
	tx := util.BuildPipelineTransaction(t.currentTx, receipt, t.currentTxFrom, baseFee)
	t.currentBlock.BlockFile.Txs = append(t.currentBlock.BlockFile.Txs, tx)

	t.callTracer = nil
	return nil
}

func (t *RPCTracer) OnLog(log *types.Log) {
	if t.callTracer != nil {
		t.callTracer.OnLog(log)
	}
}

func (t *RPCTracer) GetOutPut(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storages map[common.Hash]map[common.Hash][]byte, codes map[common.Hash][]byte) *ptypes.DebankOutPut {
	if originRoot != root {
		t.currentBlock.BlockDiff = stateUpdateToStateDiff(originRoot, root, destructs, accounts, storages, codes)
	}

	for addr := range t.currentBlock.ChangeContracts {
		t.currentBlock.BlockFile.StorageContracts = append(t.currentBlock.BlockFile.StorageContracts, strings.ToLower(addr.Hex()))
	}

	var stateDiffBytes []byte
	var encErr error
	if t.currentBlock.BlockDiff != nil {
		stateDiffBytes, encErr = util.EncodeToRlp(t.currentBlock.BlockDiff)
		if encErr != nil {
			log.Error("Failed to encode state diff", "err", encErr)
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
