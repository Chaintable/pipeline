package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

type Header struct {
	Number                *hexutil.Big     `json:"number"`
	Hash                  common.Hash      `json:"hash"`
	ParentHash            common.Hash      `json:"parentHash"`
	Nonce                 types.BlockNonce `json:"nonce"`
	MixHash               common.Hash      `json:"mixHash"`
	Sha3Uncles            common.Hash      `json:"sha3Uncles"`
	LogsBloom             types.Bloom      `json:"logsBloom"`
	StateRoot             common.Hash      `json:"stateRoot"`
	Miner                 common.Address   `json:"miner"`
	Difficulty            *hexutil.Big     `json:"difficulty"`
	ExtraData             hexutil.Bytes    `json:"extraData"`
	GasLimit              hexutil.Uint64   `json:"gasLimit"`
	GasUsed               hexutil.Uint64   `json:"gasUsed"`
	Timestamp             hexutil.Uint64   `json:"timestamp"`
	TransactionsRoot      common.Hash      `json:"transactionsRoot"`
	ReceiptsRoot          common.Hash      `json:"receiptsRoot"`
	BaseFeePerGas         *hexutil.Big     `json:"baseFeePerGas,omitempty"`
	WithdrawalsRoot       *common.Hash     `json:"withdrawalsRoot,omitempty"`
	BlobGasUsed           *hexutil.Uint64  `json:"blobGasUsed,omitempty"`
	ExcessBlobGas         *hexutil.Uint64  `json:"excessBlobGas,omitempty"`
	ParentBeaconBlockRoot *common.Hash     `json:"parentBeaconBlockRoot,omitempty"`
	RequestsRoot          *common.Hash     `json:"requestsRoot,omitempty"`
}

type Block struct {
	Header,
	Size hexutil.Uint64 `json:"size"`
	Uncles []*Header `json:"uncles"`
	// tx hash list
	Transactions []common.Hash     `json:"transactions"`
	Withdrawals  types.Withdrawals `json:withdrawals"`
	Requests     types.Requests    `json:requests"`
}
