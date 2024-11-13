package types

import (
	"math/big"
)

type WithdrawalNativeTransfer struct {
	ID           string   `json:"id"`
	Idx          *big.Int `json:"idx"`
	ValidatorIdx *big.Int `json:"validator_idx"`
	ToAddress    string   `json:"to_address"`
	Value        float64  `json:"value"` // amount / 1e18
}
