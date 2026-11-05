package tracer

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

const (
	genesisZeroAddress   = "0x0000000000000000000000000000000000000000"
	genesisNativeToken   = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	genesisTransferTx    = "0xgenesis01"
	genesisDeployTx      = "0xgenesis02"
	genesisNativeTokenTx = "0xgenesis03"
)

// GenesisOutput returns the trace_debankBlock output for the genesis block.
func GenesisOutput(block *types.Block, alloc core.GenesisAlloc) *ptypes.DebankOutPut {
	header, diff, blockFile := genesisArtifacts(block, alloc)
	stateDiff, err := util.EncodeToRlp(diff)
	if err != nil {
		log.Error("Failed to encode genesis state diff", "err", err)
		stateDiff = []byte{}
	}
	return &ptypes.DebankOutPut{
		BlockFile:      blockFile,
		Header:         header,
		StateDiff:      hexutil.Bytes(stateDiff),
		ValidationHash: blockFile.Validation().ValidationHash,
	}
}

// genesisArtifacts builds the genesis header, state diff and block file. Every
// funded account gets a synthetic transfer, every contract a synthetic
// deployment, and the native token a synthetic creation, in address order.
func genesisArtifacts(block *types.Block, alloc core.GenesisAlloc) (*ptypes.Header, *ptypes.BlockStorageDiff, *ptypes.BlockFile) {
	header := util.BuildPilelineBlockHeader(block)
	diff := GenesisAllocToStateDiff(alloc)
	diff.Hash = header.StateRoot
	blockFile := newBlockFile(block)

	addrs := make([]common.Address, 0, len(alloc))
	for addr := range alloc {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool {
		return addrs[i].Hex() < addrs[j].Hex()
	})

	txIdx := int64(0)
	add := func(prefix string, to string, input []byte, output []byte, value *big.Int, callCreateType string, callType string) {
		txID := fmt.Sprintf("%s%013d%s", prefix, 0, to)
		blockFile.Txs = append(blockFile.Txs, ptypes.Transaction{
			ID: txID, From: genesisZeroAddress, To: to,
			Gas: big.NewInt(0), GasPrice: big.NewInt(0), GasUsed: big.NewInt(0),
			Status: true, GasFeeCap: big.NewInt(0), GasTipCap: big.NewInt(0),
			Input: input, Nonce: big.NewInt(0),
			TransactionIndex: txIdx,
			Value:            (*hexutil.Big)(value),
		})
		blockFile.Traces = append(blockFile.Traces, ptypes.Trace{
			ID:   util.ToHash([]string{txID, "", "0"}),
			From: genesisZeroAddress, To: to,
			Gas: big.NewInt(0), GasUsed: big.NewInt(0),
			Input: input, Output: output,
			Value:          (*hexutil.Big)(value),
			CallCreateType: callCreateType, CallType: callType,
			TxID: txID, ParentTraceID: "", PosInParentTrace: 0,
			TraceAddress: []int64{},
		})
		txIdx++
	}
	for _, addr := range addrs {
		account := alloc[addr]
		to := strings.ToLower(addr.Hex())
		if len(account.Storage) > 0 {
			blockFile.StorageContracts = append(blockFile.StorageContracts, to)
		}
		if account.Balance != nil && account.Balance.Sign() > 0 {
			add(genesisTransferTx, to, []byte{}, []byte{}, account.Balance, "call", "call")
		}
		if len(account.Code) > 0 {
			add(genesisDeployTx, to, account.Code, account.Code, big.NewInt(0), "create", "")
		}
	}
	add(genesisNativeTokenTx, genesisNativeToken, []byte{}, []byte{}, big.NewInt(0), "create", "")
	return header, diff, blockFile
}
