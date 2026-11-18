package tracer

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/XinFinOrg/XDPoSChain/common"
	"github.com/XinFinOrg/XDPoSChain/common/hexutil"
	"github.com/XinFinOrg/XDPoSChain/core/types"
)

// genesisTxID 构造 genesis 合成 tx 的 id: 0x + 2位类型码 + 22个0 + 小写地址(去0x, 40字符), 总长66字符,
// 与真实 tx hash 等长, 且为合法 hex, 可解析为 bytes32.
// kind: 1=alloc balance transfer, 2=alloc code create, 3=native token create
func genesisTxID(kind int, addrLower string) string {
	return fmt.Sprintf("0x%02d%022d%s", kind, 0, strings.TrimPrefix(addrLower, "0x"))
}

// BuildGenesisBlockFile constructs canonical alloc and native-token records.
// It does not initialize the pipeline or upload data; RPC and live tracers share it.
func BuildGenesisBlockFile(block *types.Block, alloc ptypes.GenesisAlloc) *ptypes.BlockFile {
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

	// 构造 genesis tx 和 trace
	zeroAddr := "0x0000000000000000000000000000000000000000"
	txIdx := int64(0)

	// 对地址排序，确保遍历顺序确定性
	sortedAddrs := make([]common.Address, 0, len(alloc))
	for addr := range alloc {
		sortedAddrs = append(sortedAddrs, addr)
	}
	sort.Slice(sortedAddrs, func(i, j int) bool {
		return sortedAddrs[i].Hex() < sortedAddrs[j].Hex()
	})

	for _, addr := range sortedAddrs {
		account := alloc[addr]
		// XDC Address.Hex() uses an "xdc" prefix. Synthetic IDs and business
		// records use canonical 0x addresses regardless of the RPC prefix mode.
		addrLower := hexutil.Encode(addr[:])

		// 处理有 Storage 的账户
		if len(account.Storage) > 0 {
			blockFile.StorageContracts = append(blockFile.StorageContracts, addrLower)
		}

		// 处理有 balance 的账户 - 构造转账 tx 和 call trace
		if account.Balance != nil && account.Balance.Sign() > 0 {
			// tx id: 0x + 01 + 22个0 + 地址(去0x, 40字符) = 66字符, 可解析为 bytes32
			txID := genesisTxID(1, addrLower)

			tx := ptypes.Transaction{
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
			}
			blockFile.Txs = append(blockFile.Txs, tx)

			// trace id = hash(tx_id, parent_trace_id, pos_in_parent_trace)
			traceID := util.ToHash([]string{txID, "", "0"})
			trace := ptypes.Trace{
				ID:                traceID,
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
			}
			blockFile.Traces = append(blockFile.Traces, trace)
			txIdx++
		}

		// 处理有 code 的账户 - 构造 create tx 和 create trace
		if len(account.Code) > 0 {
			// tx id: 0x + 02 + 22个0 + 地址(去0x, 40字符) = 66字符, 可解析为 bytes32
			txID := genesisTxID(2, addrLower)

			tx := ptypes.Transaction{
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
			}
			blockFile.Txs = append(blockFile.Txs, tx)

			// trace id = hash(tx_id, parent_trace_id, pos_in_parent_trace)
			traceID := util.ToHash([]string{txID, "", "0"})
			trace := ptypes.Trace{
				ID:                traceID,
				From:              zeroAddr,
				Gas:               big.NewInt(0),
				Input:             account.Code,
				To:                addrLower,
				Value:             (*hexutil.Big)(big.NewInt(0)),
				GasUsed:           big.NewInt(0),
				Output:            account.Code, // output 直接使用 input (code)
				CallCreateType:    "create",
				CallType:          "",
				TxID:              txID,
				ParentTraceID:     "",
				PosInParentTrace:  0,
				SelfStorageChange: false,
				StorageChange:     false,
				Subtraces:         0,
				TraceAddress:      []int64{},
			}
			blockFile.Traces = append(blockFile.Traces, trace)
			txIdx++
		}
	}

	// 添加原生代币合约创建 tx 和 trace (E地址)
	nativeTokenAddr := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	// tx id: 0x + 03 + 22个0 + 地址(去0x, 40字符) = 66字符, 可解析为 bytes32
	nativeTokenTxID := genesisTxID(3, nativeTokenAddr)

	nativeTokenTx := ptypes.Transaction{
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
	}
	blockFile.Txs = append(blockFile.Txs, nativeTokenTx)

	nativeTokenTraceID := util.ToHash([]string{nativeTokenTxID, "", "0"})
	nativeTokenTrace := ptypes.Trace{
		ID:                nativeTokenTraceID,
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
	}
	blockFile.Traces = append(blockFile.Traces, nativeTokenTrace)

	return blockFile
}
