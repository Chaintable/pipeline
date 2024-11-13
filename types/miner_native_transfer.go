package types

import (
	"github.com/ethereum/go-ethereum/common"
)

type MinerNativeRransfer struct {
	ID     string         `json:"id"`
	ToAddr common.Address `json:"to_addr"`
	Value  float64        `json:"value"` //amount / 1e18
}
