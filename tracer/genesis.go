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
	"github.com/ethereum/go-ethereum/core/types"
)

func genesisTxID(kind int, addrLower string) string {
	return fmt.Sprintf("0x%02d%022d%s", kind, 0, strings.TrimPrefix(addrLower, "0x"))
}

// BuildGenesisSyntheticTransactions constructs the synthetic transactions and
// root traces represented by a genesis state.
func BuildGenesisSyntheticTransactions(finalState types.GenesisAlloc) ([]ptypes.Transaction, []ptypes.Trace) {
	txs := make([]ptypes.Transaction, 0)
	traces := make([]ptypes.Trace, 0)
	if finalState == nil {
		return txs, traces
	}
	zeroAddr := "0x0000000000000000000000000000000000000000"
	txIdx := int64(0)

	sortedAddrs := make([]common.Address, 0, len(finalState))
	for addr := range finalState {
		sortedAddrs = append(sortedAddrs, addr)
	}
	sort.Slice(sortedAddrs, func(i, j int) bool {
		return sortedAddrs[i].Hex() < sortedAddrs[j].Hex()
	})

	for _, addr := range sortedAddrs {
		account := finalState[addr]
		addrLower := strings.ToLower(addr.Hex())

		if account.Balance != nil && account.Balance.Sign() > 0 {
			txID := genesisTxID(1, addrLower)
			txs = append(txs, ptypes.Transaction{
				ID:               txID,
				From:             zeroAddr,
				To:               addrLower,
				Gas:              big.NewInt(0),
				GasPrice:         big.NewInt(0),
				GasUsed:          big.NewInt(0),
				Status:           true,
				GasFeeCap:        big.NewInt(0),
				GasTipCap:        big.NewInt(0),
				Input:            []byte{},
				Nonce:            big.NewInt(0),
				TransactionIndex: txIdx,
				Value:            (*hexutil.Big)(account.Balance),
			})

			traces = append(traces, ptypes.Trace{
				ID:                util.ToHash([]string{txID, "", "0"}),
				From:              zeroAddr,
				Gas:               big.NewInt(0),
				Input:             []byte{},
				To:                addrLower,
				Value:             (*hexutil.Big)(account.Balance),
				GasUsed:           big.NewInt(0),
				Output:            []byte{},
				CallCreateType:    "call",
				CallType:          "call",
				TxID:              txID,
				ParentTraceID:     "",
				PosInParentTrace:  0,
				SelfStorageChange: false,
				StorageChange:     false,
				Subtraces:         0,
				TraceAddress:      []int64{},
			})
			txIdx++
		}

		if len(account.Code) > 0 {
			txID := genesisTxID(2, addrLower)
			txs = append(txs, ptypes.Transaction{
				ID:               txID,
				From:             zeroAddr,
				To:               addrLower,
				Gas:              big.NewInt(0),
				GasPrice:         big.NewInt(0),
				GasUsed:          big.NewInt(0),
				Status:           true,
				GasFeeCap:        big.NewInt(0),
				GasTipCap:        big.NewInt(0),
				Input:            account.Code,
				Nonce:            big.NewInt(0),
				TransactionIndex: txIdx,
				Value:            (*hexutil.Big)(big.NewInt(0)),
			})

			traces = append(traces, ptypes.Trace{
				ID:                util.ToHash([]string{txID, "", "0"}),
				From:              zeroAddr,
				Gas:               big.NewInt(0),
				Input:             account.Code,
				To:                addrLower,
				Value:             (*hexutil.Big)(big.NewInt(0)),
				GasUsed:           big.NewInt(0),
				Output:            account.Code,
				CallCreateType:    "create",
				CallType:          "",
				TxID:              txID,
				ParentTraceID:     "",
				PosInParentTrace:  0,
				SelfStorageChange: false,
				StorageChange:     false,
				Subtraces:         0,
				TraceAddress:      []int64{},
			})
			txIdx++
		}
	}

	nativeTokenAddr := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	nativeTokenTxID := genesisTxID(3, nativeTokenAddr)
	txs = append(txs, ptypes.Transaction{
		ID:               nativeTokenTxID,
		From:             zeroAddr,
		To:               nativeTokenAddr,
		Gas:              big.NewInt(0),
		GasPrice:         big.NewInt(0),
		GasUsed:          big.NewInt(0),
		Status:           true,
		GasFeeCap:        big.NewInt(0),
		GasTipCap:        big.NewInt(0),
		Input:            []byte{},
		Nonce:            big.NewInt(0),
		TransactionIndex: txIdx,
		Value:            (*hexutil.Big)(big.NewInt(0)),
	})

	traces = append(traces, ptypes.Trace{
		ID:                util.ToHash([]string{nativeTokenTxID, "", "0"}),
		From:              zeroAddr,
		Gas:               big.NewInt(0),
		Input:             []byte{},
		To:                nativeTokenAddr,
		Value:             (*hexutil.Big)(big.NewInt(0)),
		GasUsed:           big.NewInt(0),
		Output:            []byte{},
		CallCreateType:    "create",
		CallType:          "",
		TxID:              nativeTokenTxID,
		ParentTraceID:     "",
		PosInParentTrace:  0,
		SelfStorageChange: false,
		StorageChange:     false,
		Subtraces:         0,
		TraceAddress:      []int64{},
	})

	return txs, traces
}
