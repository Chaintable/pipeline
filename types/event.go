package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Event struct {
	ID       string         `json:"id"`
	Address  common.Address `json:"contract_id"`
	Topics   []common.Hash  `json:"topics"`
	Index    uint64         `json:"idx"`
	Data     hexutil.Bytes  `json:"data"`
	TxHash   common.Hash    `json:"tx_id"`
	TraceID  string         `json:"trace_id"`
	Position *int           `json:"position_idx"`
}
