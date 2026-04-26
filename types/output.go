package types

import "github.com/mantlenetworkio/mantle/l2geth/common/hexutil"

type DebankOutPut struct {
	BlockFile      *BlockFile    `json:"block_file"`
	Header         *Header       `json:"header"`
	StateDiff      hexutil.Bytes `json:"state_diff"`
	ValidationHash int64         `json:"validation_hash"`
}
