package tracer

import (
	"math/big"
	"strings"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
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

func testAddr(b byte) *common.Address {
	a := common.BytesToAddress([]byte{b})
	return &a
}

func newTestCallTracer() *callTracer {
	t := newCallTracerRaw(make(map[common.Address]struct{}), &ptypes.BlockFile{})
	t.txID = "0xdeadbeef"
	return t
}

func findTraceByTo(t *testing.T, traces []ptypes.Trace, to *common.Address) ptypes.Trace {
	t.Helper()
	want := strings.ToLower(to.Hex())
	for _, tr := range traces {
		if tr.To == want {
			return tr
		}
	}
	t.Fatalf("trace with to=%s not found", want)
	return ptypes.Trace{}
}

// 父调用失败时,自身成功的子孙调用也应进入 ErrorTraces,并标记 "parent call failed"
func TestParentFailedChildrenMovedToErrorTraces(t *testing.T) {
	gc0 := callFrame{Type: vm.CALL, To: testAddr(3), Logs: []ptypes.Event{{Address: "0x03", LogIndex: 6}}}
	gc1 := callFrame{Type: vm.CALL, To: testAddr(4), Error: "out of gas"}
	child0 := callFrame{
		Type: vm.CALL, To: testAddr(2), Error: "execution reverted",
		Calls: []callFrame{gc0, gc1},
		Logs:  []ptypes.Event{{Address: "0x02", LogIndex: 5}},
	}
	child1 := callFrame{Type: vm.CALL, To: testAddr(5), Logs: []ptypes.Event{{Address: "0x05", LogIndex: 7}}}
	root := callFrame{Type: vm.CALL, To: testAddr(1), Calls: []callFrame{child0, child1}}

	tr := newTestCallTracer()
	tr.callstack = []callFrame{root}
	tr.OnTxEnd(nil, nil)

	bf := tr.BlockFile
	if len(bf.Traces) != 2 {
		t.Fatalf("Traces len = %d, want 2 (root, child1)", len(bf.Traces))
	}
	if got := findTraceByTo(t, bf.Traces, testAddr(1)).Error; got != "" {
		t.Errorf("root Error = %q, want empty", got)
	}
	if got := findTraceByTo(t, bf.Traces, testAddr(5)).Error; got != "" {
		t.Errorf("child1 Error = %q, want empty", got)
	}

	if len(bf.ErrorTraces) != 3 {
		t.Fatalf("ErrorTraces len = %d, want 3 (child0, gc0, gc1)", len(bf.ErrorTraces))
	}
	if got := findTraceByTo(t, bf.ErrorTraces, testAddr(2)).Error; got != "execution reverted" {
		t.Errorf("child0 Error = %q, want own error kept", got)
	}
	if got := findTraceByTo(t, bf.ErrorTraces, testAddr(3)).Error; got != "parent call failed" {
		t.Errorf("gc0 Error = %q, want \"parent call failed\"", got)
	}
	if got := findTraceByTo(t, bf.ErrorTraces, testAddr(4)).Error; got != "out of gas" {
		t.Errorf("gc1 Error = %q, want own error kept", got)
	}

	if len(bf.Events) != 1 || bf.Events[0].LogIndex != 7 {
		t.Errorf("Events = %+v, want only child1's log with LogIndex 7", bf.Events)
	}
	if len(bf.ErrorEvents) != 2 {
		t.Fatalf("ErrorEvents len = %d, want 2 (child0's, gc0's)", len(bf.ErrorEvents))
	}
	for _, e := range bf.ErrorEvents {
		if e.LogIndex != 0 {
			t.Errorf("ErrorEvent LogIndex = %d, want 0", e.LogIndex)
		}
	}
}

// 顶层调用失败时,整棵子树进入 ErrorTraces
func TestTopLevelFailedMarksWholeSubtree(t *testing.T) {
	child := callFrame{Type: vm.CALL, To: testAddr(2), Logs: []ptypes.Event{{Address: "0x02", LogIndex: 3}}}
	root := callFrame{Type: vm.CALL, To: testAddr(1), Error: "execution reverted", Calls: []callFrame{child}}

	tr := newTestCallTracer()
	tr.callstack = []callFrame{root}
	tr.OnTxEnd(nil, nil)

	bf := tr.BlockFile
	if len(bf.Traces) != 0 {
		t.Fatalf("Traces len = %d, want 0", len(bf.Traces))
	}
	if len(bf.ErrorTraces) != 2 {
		t.Fatalf("ErrorTraces len = %d, want 2", len(bf.ErrorTraces))
	}
	if got := findTraceByTo(t, bf.ErrorTraces, testAddr(1)).Error; got != "execution reverted" {
		t.Errorf("root Error = %q, want own error kept", got)
	}
	if got := findTraceByTo(t, bf.ErrorTraces, testAddr(2)).Error; got != "parent call failed" {
		t.Errorf("child Error = %q, want \"parent call failed\"", got)
	}
	if len(bf.Events) != 0 || len(bf.ErrorEvents) != 1 {
		t.Errorf("Events/ErrorEvents len = %d/%d, want 0/1", len(bf.Events), len(bf.ErrorEvents))
	}
}

// 全部成功时,不产生任何 error trace/event
func TestAllSuccessNoErrorTraces(t *testing.T) {
	gc := callFrame{Type: vm.CALL, To: testAddr(3)}
	child := callFrame{Type: vm.CALL, To: testAddr(2), Calls: []callFrame{gc}, Logs: []ptypes.Event{{Address: "0x02", LogIndex: 1}}}
	root := callFrame{Type: vm.CALL, To: testAddr(1), Calls: []callFrame{child}}

	tr := newTestCallTracer()
	tr.callstack = []callFrame{root}
	tr.OnTxEnd(nil, nil)

	bf := tr.BlockFile
	if len(bf.Traces) != 3 || len(bf.ErrorTraces) != 0 {
		t.Fatalf("Traces/ErrorTraces len = %d/%d, want 3/0", len(bf.Traces), len(bf.ErrorTraces))
	}
	for _, trace := range bf.Traces {
		if trace.Error != "" {
			t.Errorf("trace to=%s Error = %q, want empty", trace.To, trace.Error)
		}
	}
	if len(bf.Events) != 1 || len(bf.ErrorEvents) != 0 {
		t.Errorf("Events/ErrorEvents len = %d/%d, want 1/0", len(bf.Events), len(bf.ErrorEvents))
	}
}
