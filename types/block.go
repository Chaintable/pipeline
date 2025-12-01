package types

import (
	"math/big"
)

type Block struct {
	ID                    string   `json:"id,omitempty"`
	Height                *big.Int `json:"height,omitempty"`
	ParentID              string   `json:"parent_id,omitempty"`
	BaseFeePerGas         *big.Int `json:"base_fee_per_gas,omitempty"`
	Miner                 string   `json:"miner,omitempty"`
	GasLimit              *big.Int `json:"gas_limit,omitempty"`
	GasUsed               *big.Int `json:"gas_used,omitempty"`
	Timestamp             uint64   `json:"timestamp,omitempty"`
	ProcessStartTimestamp int64    `json:"process_start_timestamp,omitempty"`
}
