package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Event struct {
	Address     common.Address `json:"address"`
	Topics      []common.Hash  `json:"topics"`
	Data        hexutil.Bytes  `json:"data"`
	BlockNumber hexutil.Uint64 `json:"blockNumber"`
	TxHash      common.Hash    `json:"transactionHash"`
	TxIndex     hexutil.Uint   `json:"transactionIndex"`
	BlockHash   common.Hash    `json:"blockHash"`
	Index       hexutil.Uint   `json:"logIndex"`
	Removed     bool           `json:"removed"`

	TraceAddress []int        `json:"traceAddress"`
	Position     *uint        `json:"position"`
	GlobalIndex  hexutil.Uint `json:"globalIndex"`
}
