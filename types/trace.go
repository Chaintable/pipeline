package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Trace struct {
	ID                   string        `json:"id"`
	From                 string        `json:"from_addr"`
	Gas                  *big.Int      `json:"gas_limit"`
	Input                hexutil.Bytes `json:"input"`
	To                   string        `json:"to_addr"`
	Value                float64       `json:"value"` // double , value / 1e8
	GasUsed              *big.Int      `json:"gasUsed"`
	Output               hexutil.Bytes `json:"output"`
	Subtraces            int64         `json:"subtraces"`
	TraceAddress         string        `json:"trace_address"` // strings.Join(strArr, ", ")
	CallCreateRewardType string        `json:"type"`          // ['create', 'suicide', 'call', 'empty', 'reward']
	CallType             string        `json:"call_type"`
	RewardType           string        `json:"reward_type"`
	Error                string        `json:"error"`
	TxID                 string        `json:"tx_id"`
}
