package tracer

import (
	"math/big"
	"strings"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// An Arbitrum system tx (e.g. an L1->L2 ETH deposit) is settled by ArbOS
// without ever entering the EVM, so no OnEnter fires and the callstack is empty
// when OnTxEnd runs. Before the fix, OnTxEnd index-out-of-ranges on
// callstack[0]; in production that panic killed the executeMessages loop and
// froze the writer at the offending block. After the fix, OnTxEnd synthesizes
// the root frame from the tx so the trace is still emitted.
func TestOnTxEnd_EmptyCallstack_EmitsRootTrace(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(map[common.Address]struct{}{}, bf)

	from := common.HexToAddress("0x2222222222222222222222222222222222222222")
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tx := types.NewTx(&types.LegacyTx{To: &to, Value: big.NewInt(1e18), Gas: 21000})

	ct.OnTxStart(nil, tx, from)
	// No OnEnter: the system tx skipped the EVM call.
	ct.OnTxEnd(&types.Receipt{}, nil)

	if len(bf.Traces) != 1 {
		t.Fatalf("expected 1 trace for the EVM-less system tx, got %d", len(bf.Traces))
	}
	tr := bf.Traces[0]
	if tr.From != strings.ToLower(from.Hex()) {
		t.Errorf("trace From = %s, want %s", tr.From, strings.ToLower(from.Hex()))
	}
	if tr.To != strings.ToLower(to.Hex()) {
		t.Errorf("trace To = %s, want %s", tr.To, strings.ToLower(to.Hex()))
	}
	if tr.Value.ToInt().Cmp(big.NewInt(1e18)) != 0 {
		t.Errorf("trace Value = %s, want 1e18", tr.Value.ToInt())
	}
}

// A normal tx (OnEnter fired) must be unaffected by the fix: exactly one root
// trace, built from the real call frame.
func TestOnTxEnd_NormalTx_Unchanged(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(map[common.Address]struct{}{}, bf)

	from := common.HexToAddress("0x2222222222222222222222222222222222222222")
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tx := types.NewTx(&types.LegacyTx{To: &to, Value: big.NewInt(0), Gas: 21000})

	ct.OnTxStart(nil, tx, from)
	ct.OnEnter(0, byte(0xF1), from, to, nil, 21000, big.NewInt(0)) // 0xF1 = CALL
	ct.OnExit(0, nil, 21000, nil, false)
	ct.OnTxEnd(&types.Receipt{}, nil)

	if len(bf.Traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(bf.Traces))
	}
}
