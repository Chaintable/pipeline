package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Transaction struct {
	ID   common.Hash    `json:"id"`
	From common.Address `json:"from_addr"`
	// create的时候是合约地址
	To               *common.Address `json:"to_addr"`
	Gas              uint64          `json:"gas_limit"`
	GasPrice         *big.Int        `json:"gas_price"`
	GasUsed          hexutil.Uint64  `json:"gas_used"`
	Status           uint64          `json:"status"`
	GasFeeCap        *big.Int        `json:"max_fee_per_gas"`
	GasTipCap        *big.Int        `json:"max_priority_fee_per_gas"`
	Input            hexutil.Bytes   `json:"input"`
	Nonce            uint64          `json:"nonce"`
	TransactionIndex uint64          `json:"transaction_index"`
	Value            *big.Int        `json:"value"`
	Type             hexutil.Uint64  `json:"type"`
}
