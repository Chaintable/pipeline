package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Transaction struct {
	ID   string `json:"id"`
	From string `json:"from_addr"`
	// create的时候是合约地址
	To               string         `json:"to_addr"`
	Gas              *big.Int       `json:"gas_limit"`
	GasPrice         *big.Int       `json:"gas_price"`
	GasUsed          hexutil.Uint64 `json:"gas_used"`
	Status           bool           `json:"status"`
	GasFeeCap        *big.Int       `json:"max_fee_per_gas"`
	GasTipCap        *big.Int       `json:"max_priority_fee_per_gas"`
	Input            hexutil.Bytes  `json:"input"`
	Nonce            *big.Int       `json:"nonce"`
	TransactionIndex int64          `json:"transaction_index"`
	Value            float64        `json:"value"` // 转成小数  value/1e18
	Type             int64          `json:"type"`
}
