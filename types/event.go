package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// id = to_hash(event['parent_trace_id'], event['pos_in_parent_trace'])
type Event struct {
	ID            string        `json:"id"`
	Address       string        `json:"contract_id"`
	Selector      string        `json:"selector"`
	Topics        []string      `json:"topics"`
	Data          hexutil.Bytes `json:"data"`
	ParentTraceID string        `json:"parent_trace_id"`
	Position      int64         `json:"pos_in_parent_trace"`
	LogIndex      int64         `json:"idx"`
	// InTxLogIdx is a per-tx counter (resets to 0 at each OnTxStart, increments
	// on every OnLog). Together with the enclosing tx ID it forms a stable
	// (txID, InTxLogIdx) key that downstream canonical-events rebuild can use
	// to look up the ParentTraceID/Position/ID binding without relying on the
	// global LogIndex counter, which can disagree with iotex's receipt-side
	// log.Index allocation when actions emit logs in an interleaved order.
	InTxLogIdx int64 `json:"in_tx_log_idx"`
}
