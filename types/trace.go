package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Trace struct {
	ID                   string         `json:"id"`
	From                 common.Address `json:"from_addr"`
	Gas                  uint64         `json:"gas_limit"`
	Input                hexutil.Bytes  `json:"input"`
	To                   common.Address `json:"to_addr"`
	Value                *big.Int       `json:"value"`
	GasUsed              uint64         `json:"gasUsed"`
	Output               hexutil.Bytes  `json:"output"`
	Subtraces            int            `json:"subtraces"`
	TraceAddress         string         `json:"trace_address"` // strings.Join(strArr, ", ")
	CallCreateRewardType string         `json:"type"`          // ['create', 'suicide', 'call', 'empty', 'reward']
	CallType             string         `json:"call_type"`
	RewardType           string         `json:"reward_type"`
	Position             uint64         `json:"position_idx"`
	Error                string         `json:"error"`
}
