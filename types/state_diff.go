package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

type NewAccount struct {
	Address  common.Hash
	Balance  *uint256.Int
	Nonce    uint64
	CodeHash common.Hash
}

type NewCode struct {
	CodeHash common.Hash
	Code     []byte
}

type IndexValuePair struct {
	Index common.Hash
	Value *uint256.Int
}

type AccountStorageDiff struct {
	Address common.Hash
	Values  []IndexValuePair
}

type BlockStorageDiff struct {
	Hash            common.Hash
	ParentHash      common.Hash
	NewAccounts     []NewAccount
	DeletedAccounts []common.Hash
	StorageDiff     []AccountStorageDiff
	NewCodes        []NewCode
}

type Header struct {
	Number                *hexutil.Big     `json:"number,omitempty"`
	Hash                  common.Hash      `json:"hash,omitempty"`
	ParentHash            common.Hash      `json:"parentHash,omitempty"`
	Nonce                 types.BlockNonce `json:"nonce,omitempty"`
	MixHash               common.Hash      `json:"mixHash,omitempty"`
	Sha3Uncles            common.Hash      `json:"sha3Uncles,omitempty"`
	LogsBloom             types.Bloom      `json:"logsBloom,omitempty"`
	StateRoot             common.Hash      `json:"stateRoot,omitempty"`
	Miner                 common.Address   `json:"miner,omitempty"`
	Difficulty            *hexutil.Big     `json:"difficulty,omitempty"`
	ExtraData             hexutil.Bytes    `json:"extraData,omitempty"`
	GasLimit              hexutil.Uint64   `json:"gasLimit,omitempty"`
	GasUsed               hexutil.Uint64   `json:"gasUsed,omitempty"`
	Timestamp             hexutil.Uint64   `json:"timestamp,omitempty"`
	TransactionsRoot      common.Hash      `json:"transactionsRoot,omitempty"`
	ReceiptsRoot          common.Hash      `json:"receiptsRoot,omitempty"`
	BaseFeePerGas         *hexutil.Big     `json:"baseFeePerGas,omitempty"`
	WithdrawalsRoot       *common.Hash     `json:"withdrawalsRoot,omitempty"`
	BlobGasUsed           *hexutil.Uint64  `json:"blobGasUsed,omitempty"`
	ExcessBlobGas         *hexutil.Uint64  `json:"excessBlobGas,omitempty"`
	ParentBeaconBlockRoot *common.Hash     `json:"parentBeaconBlockRoot,omitempty"`
	RequestsRoot          *common.Hash     `json:"requestsRoot,omitempty"`
}
