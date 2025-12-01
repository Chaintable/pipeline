package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// id = to_hash(trace['tx_id'], trace['parent_trace_id'], trace['pos_in_parent_trace'])
type Trace struct {
	ID                string        `json:"id,omitempty"`
	From              string        `json:"from_addr,omitempty"`
	Gas               *big.Int      `json:"gas_limit,omitempty"`
	Input             hexutil.Bytes `json:"input,omitempty"`
	To                string        `json:"to_addr,omitempty"`
	Value             *hexutil.Big  `json:"value,omitempty"`
	GasUsed           *big.Int      `json:"gas_used,omitempty"`
	Output            hexutil.Bytes `json:"output,omitempty"`
	CallCreateType    string        `json:"type,omitempty"` // ['create', 'suicide', 'call', 'empty']
	CallType          string        `json:"call_type,omitempty"`
	TxID              string        `json:"tx_id,omitempty"`
	ParentTraceID     string        `json:"parent_trace_id,omitempty"`
	PosInParentTrace  int64         `json:"pos_in_parent_trace,omitempty"`
	SelfStorageChange bool          `json:"self_storage_change,omitempty"`
	StorageChange     bool          `json:"storage_change,omitempty"`
	Subtraces         int64         `json:"subtraces,omitempty"`
	TraceAddress      []int64       `json:"trace_address,omitempty"`
	Error             string        `json:"error,omitempty"`
}
