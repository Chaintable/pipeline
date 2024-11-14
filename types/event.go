package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// id = to_hash(event['parent_trace_id'], event['pos_in_parent_trace'])
type Event struct {
	ID            string        `json:"id"`
	Address       string        `json:"contract_id"`
	Topics        []string      `json:"topics"`
	Data          hexutil.Bytes `json:"data"`
	TxHash        string        `json:"tx_id"`
	ParentTraceID string        `json:"parent_trace_id"`
	Position      int64         `json:"pos_in_parent_trace"`
}
