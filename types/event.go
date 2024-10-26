package types

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Event struct {
	Address     common.Address `json:"address"`
	Topics      []common.Hash  `json:"topics"`
	Data        hexutil.Bytes  `json:"data"`
	BlockNumber hexutil.Uint64 `json:"blockNumber"`
	BlockHash   common.Hash    `json:"blockHash"`
	TxHash      common.Hash    `json:"transactionHash"`
	TxIndex     hexutil.Uint   `json:"transactionIndex"`
	Index       hexutil.Uint   `json:"logIndex"`
	Removed     bool           `json:"removed"`

	TraceAddress []int        `json:"traceAddress"`
	Position     *int         `json:"position"`
	GlobalIndex  hexutil.Uint `json:"globalIndex"`
}

type EventPosition struct {
	Index        uint  `json:"logIndex"`
	TraceAddress []int `json:"traceAddress"`
	Position     *int  `json:"position"`
	GlobalIndex  uint  `json:"globalIndex"`
}

type EventHash struct {
	Address      common.Address `json:"address"`
	Topics       []common.Hash  `json:"topics"`
	Data         hexutil.Bytes  `json:"data"`
	TxHash       common.Hash    `json:"transactionHash"`
	Index        hexutil.Uint   `json:"logIndex"`
	TraceAddress []int          `json:"traceAddress"`
	Position     *int           `json:"position"`
	GlobalIndex  hexutil.Uint   `json:"globalIndex"`
}

func (e *Event) Hash() common.Hash {
	h := EventHash{
		Address:      e.Address,
		Topics:       e.Topics,
		Data:         e.Data,
		TxHash:       e.TxHash,
		Index:        e.Index,
		TraceAddress: e.TraceAddress,
		Position:     e.Position,
		GlobalIndex:  e.GlobalIndex,
	}
	buf, _ := json.Marshal(h)
	hash := sha256.Sum256(buf)
	return common.BytesToHash(hash[:])
}
