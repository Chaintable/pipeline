package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Event struct {
	ID       string        `json:"id"`
	Address  string        `json:"contract_id"`
	Topics   []string      `json:"topics"`
	Index    int64         `json:"idx"`
	Data     hexutil.Bytes `json:"data"`
	TxHash   string        `json:"tx_id"`
	TraceID  string        `json:"trace_id"`
	Position int64         `json:"position_idx"`
}
