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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

type RPCTracer struct {
	callTracer     *callTracer
	currentBlock   *RPCBlockContext
	prestateTracer *prestateTracer
}

type RPCBlockContext struct {
	BlockDiff       *ptypes.BlockStorageDiff
	BlockHeader     *ptypes.Header
	BlockFile       *ptypes.BlockFile
	Tx              *types.Transaction
	From            common.Address
	ChangeContracts map[common.Address]struct{}
}

func (t *RPCTracer) Stop(err error) {
	return
}

func (t *RPCTracer) GetResult() (json.RawMessage, error) {
	return nil, nil
}

func (t *RPCTracer) OnBlockStart(block *types.Block, chainConfig *params.ChainConfig) {
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
	t.prestateTracer = newPrestateTracer(&prestateTracerConfig{
		DiffMode: true,
	}, chainConfig)
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

func (t *RPCTracer) OnOpcode(pc uint64, opcode byte, gas, cost uint64, scope tracing.OpContext, rData []byte, depth int, err error) {
	if t.callTracer != nil {
		t.callTracer.OnOpcode(pc, opcode, gas, cost, scope, rData, depth, err)
	}
	t.prestateTracer.OnOpcode(pc, opcode, gas, cost, scope, rData, depth, err)

}

func (t *RPCTracer) OnTxStart(env *tracing.VMContext, tx *types.Transaction, from common.Address) {
	callTracer := newCallTracerRaw(t.currentBlock.ChangeContracts, t.currentBlock.BlockFile)
	t.callTracer = callTracer
	t.callTracer.OnTxStart(env, tx, from)

	t.prestateTracer.OnTxStart(env, tx, from)

	t.currentBlock.From = from
	t.currentBlock.Tx = tx
}

func (t *RPCTracer) OnTxEnd(receipt *types.Receipt, err error) {
	t.callTracer.OnTxEnd(receipt, err)
	t.callTracer = nil

	t.prestateTracer.OnTxEnd(receipt, err)

	tx := util.BuildPipelineTransaction(t.currentBlock.Tx, receipt, t.currentBlock.From, t.currentBlock.BlockHeader.BaseFeePerGas.ToInt())
	t.currentBlock.BlockFile.Txs = append(t.currentBlock.BlockFile.Txs, tx)
}

func (t *RPCTracer) OnLog(log *types.Log) {
	if t.callTracer != nil {
		t.callTracer.OnLog(log)
	}
}

func (t *RPCTracer) GetOutPut(originRoot common.Hash, root common.Hash) *ptypes.DebankOutPut {
	if originRoot != root {
		t.currentBlock.BlockDiff = t.GetStateDiff(originRoot, root)
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

func (t *RPCTracer) GetStateDiff(originRoot common.Hash, root common.Hash) *ptypes.BlockStorageDiff {
	stateDiff := &ptypes.BlockStorageDiff{}
	if originRoot == (common.Hash{}) {
		originRoot = types.EmptyRootHash
	}
	if root == (common.Hash{}) {
		root = types.EmptyRootHash
	}
	stateDiff.Hash = root
	stateDiff.ParentHash = originRoot

	for addr, newAccount := range t.prestateTracer.post {
		oldAccount, exists := t.prestateTracer.pre[addr]
		if !exists {
			// If the account does not exist in prestate, it is a new create account
			oldAccount = &account{
				Balance: big.NewInt(0),
				Nonce:   0,
				empty:   true,
			}
		}

		// only storage changes
		if newAccount.Nonce == 0 && len(newAccount.Code) == 0 && newAccount.Balance == nil {
			continue
		}
		if newAccount.Balance != nil || newAccount.Nonce != 0 || len(newAccount.Code) > 0 {
			newBalance := oldAccount.Balance
			if newAccount.Balance != nil {
				newBalance = newAccount.Balance
			}
			newNonce := oldAccount.Nonce
			if newAccount.Nonce != 0 {
				newNonce = newAccount.Nonce
			}
			newCodeHash := crypto.Keccak256Hash(oldAccount.Code)
			if len(newAccount.Code) > 0 {
				newCodeHash = crypto.Keccak256Hash(newAccount.Code)
			}

			stateDiff.NewAccounts = append(stateDiff.NewAccounts, ptypes.NewAccount{
				Address:  addressToHash(addr),
				Balance:  uint256.MustFromBig(newBalance),
				Nonce:    newNonce,
				CodeHash: newCodeHash,
			})
		}
	}

	for addr := range t.prestateTracer.deleted {
		stateDiff.DeletedAccounts = append(stateDiff.DeletedAccounts, addressToHash(addr))
	}

	for addr, acct := range t.prestateTracer.post {
		Values := make([]ptypes.IndexValuePair, 0, len(acct.Storage))
		for k, v := range acct.Storage {
			value := uint256.NewInt(0).SetBytes(v.Bytes())
			Values = append(Values, ptypes.IndexValuePair{
				Index: crypto.Keccak256Hash(k[:]),
				Value: value,
			})
		}
		if len(Values) > 0 {
			stateDiff.StorageDiff = append(stateDiff.StorageDiff, ptypes.AccountStorageDiff{
				Address: addressToHash(addr),
				Values:  Values,
			})
		}
	}

	for _, code := range t.prestateTracer.post {
		if len(code.Code) > 0 {
			stateDiff.NewCodes = append(stateDiff.NewCodes, ptypes.NewCode{
				CodeHash: crypto.Keccak256Hash(code.Code),
				Code:     code.Code,
			})
		}
	}
	return stateDiff
}
