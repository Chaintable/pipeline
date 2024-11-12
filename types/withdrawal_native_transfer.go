package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type WithdrawalNativeTransfer struct {
	ID           string         `json:"id"`
	Idx          uint64         `json:"idx"`
	ValidatorIdx uint64         `json:"validator_idx"`
	ToAddress    common.Address `json:"to_address"`
	Value        *big.Int       `json:"value"`
}
