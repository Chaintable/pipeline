package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

type Receipt struct {
	BlockHash         common.Hash     `json:"blockHash"`
	BlockNumber       hexutil.Uint64  `json:"blockNumber"`
	TransactionHash   common.Hash     `json:"transactionHash"`
	TransactionIndex  hexutil.Uint64  `json:"transactionIndex"`
	From              common.Address  `json:"from"`
	To                *common.Address `json:"to"`
	GasUsed           hexutil.Uint64  `json:"gasUsed"`
	CumulativeGasUsed hexutil.Uint64  `json:"cumulativeGasUsed"`
	ContractAddress   *common.Address `json:"contractAddress,omitempty"`
	LogsBloom         types.Bloom     `json:"logsBloom"`
	EffectiveGasPrice *hexutil.Big    `json:"effectiveGasPrice,omitempty"`
	Type              hexutil.Uint    `json:"type"`
	Status            *hexutil.Uint   `json:"status,omitempty"`
	Root              *hexutil.Bytes  `json:"root,omitempty"`
	BlobGasUsed       *hexutil.Uint64 `json:"blobGasUsed,omitempty"`
	BlobGasPrice      *hexutil.Big    `json:"blobGasPrice,omitempty"`
	EventIDs          []common.Hash   `json:"eventIDs,omitempty"`
	TraceIDs          []common.Hash   `json:"traceIDs,omitempty"`

	// op 字段
	L1GasPrice            *hexutil.Big    `json:"l1GasPrice,omitempty"`
	L1GasUsed             *hexutil.Big    `json:"l1GasUsed,omitempty"`
	L1Fee                 *hexutil.Big    `json:"l1Fee,omitempty"`
	L1FeeScalar           *string         `json:"l1FeeScalar,omitempty"`
	L1BlobBaseFee         *hexutil.Big    `json:"l1BlobBaseFee,omitempty"`
	L1BaseFeeScalar       *hexutil.Uint64 `json:"l1BaseFeeScalar,omitempty"`
	L1BlobBaseFeeScalar   *hexutil.Uint64 `json:"l1BlobBaseFeeScalar,omitempty"`
	DepositNonce          *hexutil.Uint64 `json:"depositNonce,omitempty"`
	DepositReceiptVersion *hexutil.Uint64 `json:"depositReceiptVersion,omitempty"`
}
