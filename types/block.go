package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Block struct {
	ID                    string   `json:"id"`
	Height                *big.Int `json:"height"`
	ParentID              string   `json:"parent_id"`
	BaseFeePerGas         *big.Int `json:"base_fee_per_gas"`
	Miner                 string   `json:"miner"`
	GasLimit              *big.Int `json:"gas_limit"`
	GasUsed               *big.Int `json:"gas_used"`
	Timestamp             uint64   `json:"timestamp"`
	ProcessStartTimestamp int64    `json:"process_start_timestamp"`
}

type EvmHeader struct {
	Number     *big.Int
	Hash       common.Hash
	ParentHash common.Hash
	Root       common.Hash
	TxHash     common.Hash
	Time       uint64
	Coinbase   common.Address

	GasLimit uint64
	GasUsed  uint64

	BaseFee *big.Int
}
