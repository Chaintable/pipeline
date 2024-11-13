package types

import (
	"math/big"
)

type Block struct {
	ID            string   `json:"id"`
	Height        *big.Int `json:"height"`
	ParentID      string   `json:"parent_id"`
	BaseFeePerGas *big.Int `json:"base_fee_per_gas,omitempty"`
	Miner         string   `json:"miner"`
	TimeAt        float64  `json:"time_at"`
}
