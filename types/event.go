package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// id = to_hash(event['parent_trace_id'], event['pos_in_parent_trace'])
type Event struct {
	ID            string        `json:"id,omitempty"`
	Address       string        `json:"contract_id,omitempty"`
	Selector      string        `json:"selector,omitempty"`
	Topics        []string      `json:"topics,omitempty"`
	Data          hexutil.Bytes `json:"data,omitempty"`
	ParentTraceID string        `json:"parent_trace_id,omitempty"`
	Position      int64         `json:"pos_in_parent_trace,omitempty"`
	LogIndex      int64         `json:"idx,omitempty"`
}
