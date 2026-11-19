package tracer

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/morph-l2/go-ethereum/common"
	"github.com/morph-l2/go-ethereum/core/types"
	"github.com/morph-l2/go-ethereum/core/vm"
)

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
	tr.roots = []callFrame{root}
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
	tr.roots = []callFrame{root}
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
	tr.roots = []callFrame{root}
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

// SELFDESTRUCT uses the same failed-parent classification as other child frames.
func TestSelfDestructUnderFailedParentIsErrorTrace(t *testing.T) {
	selfdestruct := callFrame{Type: vm.SELFDESTRUCT, To: testAddr(3)}
	failed := callFrame{
		Type:  vm.CALL,
		To:    testAddr(2),
		Error: "execution reverted",
		Calls: []callFrame{selfdestruct},
	}
	root := callFrame{Type: vm.CALL, To: testAddr(1), Calls: []callFrame{failed}}

	tr := newTestCallTracer()
	tr.roots = []callFrame{root}
	tr.OnTxEnd(nil, nil)

	bf := tr.BlockFile
	if len(bf.Traces) != 1 || len(bf.ErrorTraces) != 2 {
		t.Fatalf(
			"Traces/ErrorTraces len = %d/%d, want 1/2",
			len(bf.Traces),
			len(bf.ErrorTraces),
		)
	}
	got := findTraceByTo(t, bf.ErrorTraces, testAddr(3))
	if got.CallCreateType != "suicide" {
		t.Errorf("SELFDESTRUCT CallCreateType = %q, want suicide", got.CallCreateType)
	}
	if got.Error != "parent call failed" {
		t.Errorf("SELFDESTRUCT Error = %q, want parent call failed", got.Error)
	}
	if len(got.TraceAddress) != 2 || got.TraceAddress[0] != 0 || got.TraceAddress[1] != 0 {
		t.Errorf("SELFDESTRUCT TraceAddress = %v, want [0 0]", got.TraceAddress)
	}
}

func TestMultipleTopLevelCalls(t *testing.T) {
	tr := newTestCallTracer()
	for i, op := range []vm.OpCode{vm.STATICCALL, vm.CALL, vm.CALL, vm.CALL} {
		to := *testAddr(byte(i + 1))
		tr.OnEnter(0, byte(op), common.Address{}, to, nil, 100, nil)
		if i != 0 {
			tr.OnLog(&types.Log{Address: to, Index: uint(i)})
			tr.OnOpcode(0, byte(vm.SSTORE), 0, 0, nil, nil, 1, nil)
		}
		var err error
		if i == 2 {
			err = vm.ErrExecutionReverted
		}
		tr.OnExit(0, []byte{byte(i)}, uint64(i+1), err, err != nil)
	}
	tr.OnTxEnd(&types.Receipt{Status: types.ReceiptStatusFailed}, nil)
	b := tr.BlockFile
	if len(b.Traces) != 4 || len(b.ErrorTraces) != 1 || len(b.Events) != 2 || len(b.ErrorEvents) != 1 {
		t.Fatalf("unexpected block file: %+v", b)
	}
	root := b.Traces[0]
	if root.CallCreateType != "empty" || root.CallType != "" || root.ParentTraceID != "" ||
		root.Subtraces != 4 || root.SelfStorageChange || !root.StorageChange ||
		root.ID != util.ToHash([]string{tr.txID, "", "0"}) || len(root.TraceAddress) != 0 {
		t.Fatalf("root: %+v", root)
	}
	for i, child := range []ptypes.Trace{b.Traces[1], b.Traces[2], b.ErrorTraces[0], b.Traces[3]} {
		if child.ParentTraceID != root.ID || child.PosInParentTrace != int64(i) ||
			!slices.Equal(child.TraceAddress, []int64{int64(i)}) ||
			child.GasUsed.Uint64() != uint64(i+1) || !bytes.Equal(child.Output, []byte{byte(i)}) {
			t.Fatalf("child %d: %+v", i, child)
		}
	}
	if b.Traces[1].CallType != "staticcall" || b.Events[0].ParentTraceID != b.Traces[2].ID ||
		b.Events[1].ParentTraceID != b.Traces[3].ID || b.ErrorEvents[0].ParentTraceID != b.ErrorTraces[0].ID {
		t.Fatalf("incorrect fee or business frames: %+v", b)
	}
}

func TestTxEndWithoutCompleteTree(t *testing.T) {
	tr := newTestCallTracer()
	tr.OnTxEnd(nil, nil)
	tr.OnEnter(0, byte(vm.CALL), common.Address{}, common.Address{}, nil, 0, nil)
	tr.OnExit(0, nil, 0, nil, false)
	tr.OnEnter(0, byte(vm.CALL), common.Address{}, common.Address{}, nil, 0, nil)
	tr.OnTxEnd(nil, nil)
	if len(tr.BlockFile.Traces)+len(tr.BlockFile.ErrorTraces) != 0 {
		t.Fatal("emitted partial tree")
	}
}
