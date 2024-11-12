package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type MinerNativeRransfer struct {
	ID     string         `json:"id"`
	ToAddr common.Address `json:"to_addr"`
	Value  *big.Int       `json:"value"`
}
