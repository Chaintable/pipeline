package tracer

import (
	"math/big"
	"strings"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// A tx that never enters the EVM (e.g. one reverted by the Arbitrum onchain tx
// filter's RevertedTxHook) fires no OnEnter, so the callstack is empty when
// OnTxEnd runs. Before the fix, OnTxEnd index-out-of-ranged on callstack[0] and,
// under nitro's StopWaiter, froze the writer. After the fix, OnTxEnd synthesizes
// the root frame from the tx + receipt: a failed receipt lands it in ErrorTraces
// with the real gas used.
func TestOnTxEnd_EmptyCallstack_FilteredTx_ErrorTrace(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(map[common.Address]struct{}{}, bf)

	from := common.HexToAddress("0xbe16fa8fc038e9637fdc55eda9fe278c9e6442a7")
	to := common.HexToAddress("0x3da71fe11585b7c9173a448d340c1758b5f6c0c2")
	tx := types.NewTx(&types.LegacyTx{To: &to, Value: big.NewInt(1e18), Gas: 100000})

	ct.OnTxStart(nil, tx, from)
	// No OnEnter: the filtered tx skipped the EVM. Receipt is failed, all gas used.
	ct.OnTxEnd(&types.Receipt{Status: types.ReceiptStatusFailed, GasUsed: 100000}, nil)

	if len(bf.ErrorTraces) != 1 {
		t.Fatalf("expected 1 error trace for the filtered tx, got error_traces=%d traces=%d", len(bf.ErrorTraces), len(bf.Traces))
	}
	if len(bf.Traces) != 0 {
		t.Fatalf("a failed/filtered tx must not land in successful traces, got %d", len(bf.Traces))
	}
	tr := bf.ErrorTraces[0]
	if tr.GasUsed.Uint64() != 100000 {
		t.Errorf("trace GasUsed = %s, want 100000 (from receipt)", tr.GasUsed)
	}
	if tr.Error == "" {
		t.Errorf("filtered/failed tx trace must be marked failed (Error set), got empty")
	}
	if tr.From != strings.ToLower(from.Hex()) {
		t.Errorf("trace From = %s, want %s", tr.From, strings.ToLower(from.Hex()))
	}
	if tr.To != strings.ToLower(to.Hex()) {
		t.Errorf("trace To = %s, want %s", tr.To, strings.ToLower(to.Hex()))
	}
}

// A successful EVM-less tx synthesizes a non-failed root trace with the receipt gas.
func TestOnTxEnd_EmptyCallstack_SuccessTx_Trace(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(map[common.Address]struct{}{}, bf)
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tx := types.NewTx(&types.LegacyTx{To: &to, Value: big.NewInt(0), Gas: 21000})

	ct.OnTxStart(nil, tx, common.HexToAddress("0x2222222222222222222222222222222222222222"))
	ct.OnTxEnd(&types.Receipt{Status: types.ReceiptStatusSuccessful, GasUsed: 21000}, nil)

	if len(bf.Traces) != 1 || len(bf.ErrorTraces) != 0 {
		t.Fatalf("expected 1 successful trace, got traces=%d error_traces=%d", len(bf.Traces), len(bf.ErrorTraces))
	}
	if bf.Traces[0].GasUsed.Uint64() != 21000 {
		t.Errorf("trace GasUsed = %s, want 21000 (from receipt)", bf.Traces[0].GasUsed)
	}
}

// A normal tx (OnEnter fired) is unaffected by the synthesis path.
func TestOnTxEnd_NormalTx_Unchanged(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(map[common.Address]struct{}{}, bf)
	from := common.HexToAddress("0x2222222222222222222222222222222222222222")
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tx := types.NewTx(&types.LegacyTx{To: &to, Value: big.NewInt(0), Gas: 21000})

	ct.OnTxStart(nil, tx, from)
	ct.OnEnter(0, byte(0xF1), from, to, nil, 21000, big.NewInt(0)) // 0xF1 = CALL
	ct.OnExit(0, nil, 21000, nil, false)
	ct.OnTxEnd(&types.Receipt{Status: types.ReceiptStatusSuccessful, GasUsed: 21000}, nil)

	if len(bf.Traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(bf.Traces))
	}
}
