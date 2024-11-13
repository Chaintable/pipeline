package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type WithdrawalNativeTransfer struct {
	ID           string         `json:"id"`
	Idx          int64          `json:"idx"`
	ValidatorIdx *big.Int       `json:"validator_idx"`
	ToAddress    common.Address `json:"to_address"`
	Value        float64        `json:"value"` // amount / 1e18
}
