package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Transaction struct {
	ID               string        `json:"id,omitempty"`
	From             string        `json:"from_addr,omitempty"`
	To               string        `json:"to_addr,omitempty"` // create的时候是合约地址
	Gas              *big.Int      `json:"gas_limit,omitempty"`
	GasPrice         *big.Int      `json:"gas_price,omitempty"`
	GasUsed          *big.Int      `json:"gas_used,omitempty"`
	Status           bool          `json:"status,omitempty"`
	GasFeeCap        *big.Int      `json:"max_fee_per_gas,omitempty"`
	GasTipCap        *big.Int      `json:"max_priority_fee_per_gas,omitempty"`
	Input            hexutil.Bytes `json:"input,omitempty"`
	Nonce            *big.Int      `json:"nonce,omitempty"`
	TransactionIndex int64         `json:"idx,omitempty"`
	Value            *hexutil.Big  `json:"value,omitempty"`
}
