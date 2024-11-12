package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Block struct {
	ID            common.Hash    `json:"id"`
	Height        uint64         `json:"height"`
	ParentID      common.Hash    `json:"parent_id"`
	BaseFeePerGas *big.Int       `json:"base_fee_per_gas,omitempty"`
	Miner         common.Address `json:"miner"`
	TimeAt        uint64         `json:"time_at"`
}
