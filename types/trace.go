package types

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type CallFrame struct {
	Action              CallAction   `json:"action"`
	BlockHash           *common.Hash `json:"blockHash"`
	BlockNumber         uint64       `json:"blockNumber"`
	Error               string       `json:"error,omitempty"`
	Result              *CallResult  `json:"result,omitempty"`
	Subtraces           int          `json:"subtraces"`
	TraceAddress        []int        `json:"traceAddress"`
	TransactionHash     *common.Hash `json:"transactionHash"`
	TransactionPosition uint64       `json:"transactionPosition"`
	Type                string       `json:"type"`
	Index               uint64       `json:"traceIndex"`
}

type CallAction struct {
	Author         *common.Address `json:"author,omitempty"`
	RewardType     string          `json:"rewardType,omitempty"`
	SelfDestructed *common.Address `json:"address,omitempty"`
	Balance        *hexutil.Big    `json:"balance,omitempty"`
	CallType       string          `json:"callType,omitempty"`
	CreationMethod string          `json:"creationMethod,omitempty"`
	From           *common.Address `json:"from,omitempty"`
	Gas            *hexutil.Uint64 `json:"gas,omitempty"`
	Init           *hexutil.Bytes  `json:"init,omitempty"`
	Input          *hexutil.Bytes  `json:"input,omitempty"`
	RefundAddress  *common.Address `json:"refundAddress,omitempty"`
	To             *common.Address `json:"to,omitempty"`
	Value          *hexutil.Big    `json:"value,omitempty"`
}

type CallResult struct {
	Address *common.Address `json:"address,omitempty"`
	Code    *hexutil.Bytes  `json:"code,omitempty"`
	GasUsed *hexutil.Uint64 `json:"gasUsed,omitempty"`
	Output  *hexutil.Bytes  `json:"output,omitempty"`
}

type CallFrameHash struct {
	TransactionHash *common.Hash    `json:"transactionHash,omitempty"`
	TraceAddress    []int           `json:"traceAddress,omitempty"`
	Subtraces       int             `json:"subtraces"`
	Index           uint64          `json:"index"`
	CallType        string          `json:"callType,omitempty"`
	From            *common.Address `json:"from,omitempty"`
	To              *common.Address `json:"to,omitempty"`
	Init            *hexutil.Bytes  `json:"init,omitempty"`
	Input           *hexutil.Bytes  `json:"input,omitempty"`
	Value           *hexutil.Big    `json:"value,omitempty"`
	Code            *hexutil.Bytes  `json:"code,omitempty"`
	GasUsed         *hexutil.Uint64 `json:"gasUsed,omitempty"`
	Output          *hexutil.Bytes  `json:"output,omitempty"`
}

func (c *CallFrame) Hash() common.Hash {
	h := CallFrameHash{
		TransactionHash: c.TransactionHash,
		TraceAddress:    c.TraceAddress,
		Subtraces:       c.Subtraces,
		Index:           c.Index,
		CallType:        c.Action.CallType,
		From:            c.Action.From,
		To:              c.Action.To,
		Init:            c.Action.Init,
		Input:           c.Action.Input,
		Value:           c.Action.Value,
		Code:            c.Result.Code,
		GasUsed:         c.Result.GasUsed,
		Output:          c.Result.Output,
	}
	buf, _ := json.Marshal(h)
	hash := sha256.Sum256(buf)
	return common.BytesToHash(hash[:])
}
