package tracer

import (
	"math/big"
	"reflect"
	"strings"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/core/vm"
)

func testAddr(b byte) *common.Address {
	a := common.BytesToAddress([]byte{b})
	return &a
}

func traceTestTree(root callFrame) *ptypes.BlockFile {
	tr := newCallTracerRaw(make(map[common.Address]struct{}), &ptypes.BlockFile{})
	tr.txID = "0xdeadbeef"
	tr.callstack = []callFrame{root}
	tr.OnTxEnd(nil, nil)
	return tr.BlockFile
}

func findTraceByTo(t *testing.T, traces []ptypes.Trace, to *common.Address) ptypes.Trace {
	t.Helper()
	want := strings.ToLower(to.Hex())
	for _, trace := range traces {
		if trace.To == want {
			return trace
		}
	}
	t.Fatalf("trace with to=%s not found", want)
	return ptypes.Trace{}
}

func TestParentFailedChildrenMovedToErrorTraces(t *testing.T) {
	root := callFrame{Type: vm.CALL, To: testAddr(1), Calls: []callFrame{
		{
			Type: vm.CALL, To: testAddr(2), Error: "execution reverted", RevertReason: "denied",
			Calls: []callFrame{
				{Type: vm.CALL, To: testAddr(3), Logs: []ptypes.Event{{Address: "0x03", LogIndex: 6}}},
				{Type: vm.CALL, To: testAddr(4), Error: "out of gas", PosInParentTrace: 1},
				{Type: vm.SELFDESTRUCT, To: testAddr(6), PosInParentTrace: 2},
			},
			Logs: []ptypes.Event{{Address: "0x02", Position: 3, LogIndex: 5}},
		},
		{Type: vm.CALL, To: testAddr(5), PosInParentTrace: 1, Logs: []ptypes.Event{{Address: "0x05", LogIndex: 7}}},
	}}
	bf := traceTestTree(root)
	if len(bf.Traces) != 2 || len(bf.ErrorTraces) != 4 {
		t.Fatalf("traces/error traces = %d/%d, want 2/4", len(bf.Traces), len(bf.ErrorTraces))
	}
	for _, addr := range []byte{1, 5} {
		if got := findTraceByTo(t, bf.Traces, testAddr(addr)).Error; got != "" {
			t.Errorf("successful root/sibling error = %q", got)
		}
	}
	for addr, want := range map[byte]string{2: "execution reverted: denied", 3: "parent call failed", 4: "out of gas", 6: "parent call failed"} {
		if got := findTraceByTo(t, bf.ErrorTraces, testAddr(addr)).Error; got != want {
			t.Errorf("trace %d error = %q, want %q", addr, got, want)
		}
	}
	if len(bf.Events) != 1 || bf.Events[0].LogIndex != 7 || len(bf.ErrorEvents) != 2 {
		t.Fatalf("unexpected events/error events: %+v / %+v", bf.Events, bf.ErrorEvents)
	}
	for _, event := range bf.ErrorEvents {
		if event.LogIndex != 0 {
			t.Errorf("reverted log index = %d, want 0", event.LogIndex)
		}
	}
}

func TestParentFailurePreservesOtherTraceFields(t *testing.T) {
	newTree := func(rootError string) callFrame {
		return callFrame{Type: vm.CALL, To: testAddr(1), Error: rootError, Calls: []callFrame{
			{
				Type: vm.CALL, From: *testAddr(1), To: testAddr(2), Gas: 100, GasUsed: 30,
				Input: []byte{1, 2}, Output: []byte{3, 4}, Value: big.NewInt(7), SelfStorageChange: true,
				Logs:  []ptypes.Event{{Address: "0x02", Position: 1, LogIndex: 3}},
				Calls: []callFrame{{Type: vm.SELFDESTRUCT, From: *testAddr(2), To: testAddr(3), Value: big.NewInt(9)}},
			},
		}}
	}
	success := traceTestTree(newTree(""))
	failed := traceTestTree(newTree("execution reverted"))
	if len(success.Traces) != 3 || len(success.ErrorTraces) != 0 || len(failed.Traces) != 0 || len(failed.ErrorTraces) != 3 {
		t.Fatal("unexpected trace classification")
	}
	for _, addr := range []byte{2, 3} {
		want := findTraceByTo(t, success.Traces, testAddr(addr))
		want.Error = "parent call failed"
		got := findTraceByTo(t, failed.ErrorTraces, testAddr(addr))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("trace %d changed beyond error: got %+v, want %+v", addr, got, want)
		}
	}
	if len(success.Events) != 1 || len(success.ErrorEvents) != 0 || len(failed.Events) != 0 || len(failed.ErrorEvents) != 1 {
		t.Fatal("unexpected event classification")
	}
	wantEvent := success.Events[0]
	wantEvent.LogIndex = 0
	if !reflect.DeepEqual(failed.ErrorEvents[0], wantEvent) {
		t.Fatal("reverted event changed beyond log index")
	}
	if !reflect.DeepEqual(success.StorageContracts, failed.StorageContracts) {
		t.Fatal("storage contracts changed")
	}
}
