package tracer

import (
	"math/big"
	"reflect"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

// Emit the same enter/exit hooks as SELFDESTRUCT. Its own frame and the
// containing call succeed; an outer call can subsequently revert.
func traceSelfDestruct(depth int, precedingCall bool, failedAncestor int) *ptypes.BlockFile {
	tr := newTestCallTracer()
	for d := 0; d <= depth; d++ {
		tr.OnEnter(d, byte(vm.CALL), *testAddr(byte(d)), *testAddr(byte(d + 1)), nil, 100000, big.NewInt(0))
	}
	contract := *testAddr(byte(depth + 1))
	if precedingCall {
		tr.OnEnter(depth+1, byte(vm.CALL), contract, *testAddr(50), nil, 10000, big.NewInt(0))
		tr.OnExit(depth+1, nil, 100, nil, false)
		tr.OnLog(&types.Log{Address: contract, Index: 3})
	}
	tr.OnEnter(depth+1, byte(vm.SELFDESTRUCT), contract, *testAddr(99), nil, 0, big.NewInt(42))
	tr.OnExit(depth+1, nil, 0, nil, false)
	for d := depth; d >= 0; d-- {
		if d == failedAncestor {
			tr.OnExit(d, nil, 1000, vm.ErrExecutionReverted, true)
		} else {
			tr.OnExit(d, nil, 1000, nil, false)
		}
	}
	tr.OnTxEnd(nil, nil)
	return tr.BlockFile
}

func TestSelfDestructTraceInheritanceAndAddress(t *testing.T) {
	for _, tc := range []struct {
		name           string
		depth          int
		precedingCall  bool
		failedAncestor int
		address        []int64
	}{
		{"root", 0, false, -1, []int64{0}},
		{"after_call_and_log", 0, true, -1, []int64{1}},
		{"nested", 2, false, -1, []int64{0, 0, 0}},
		{"reverted_parent", 2, false, 1, []int64{0, 0, 0}},
		{"reverted_root_after_call_and_log", 2, true, 0, []int64{0, 0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bf := traceSelfDestruct(tc.depth, tc.precedingCall, tc.failedAncestor)
			traces, otherTraces := bf.Traces, bf.ErrorTraces
			wantError := ""
			if tc.failedAncestor >= 0 {
				traces, otherTraces = bf.ErrorTraces, bf.Traces
				wantError = "parent call failed"
			}
			got := findTraceByTo(t, traces, testAddr(99))
			for _, tr := range otherTraces {
				if tr.ID == got.ID {
					t.Fatal("SELFDESTRUCT appears in both trace lists")
				}
			}
			if got.CallCreateType != "suicide" || got.Error != wantError {
				t.Fatalf("SELFDESTRUCT type/error = %q/%q, want suicide/%q", got.CallCreateType, got.Error, wantError)
			}
			if !reflect.DeepEqual(got.TraceAddress, tc.address) {
				t.Errorf("trace address = %v, want %v", got.TraceAddress, tc.address)
			}
			wantPosition := int64(0)
			if tc.precedingCall {
				wantPosition = 2 // one call and one log precede SELFDESTRUCT
			}
			if got.PosInParentTrace != wantPosition || got.Subtraces != 0 {
				t.Errorf("position/subtraces = %d/%d, want %d/0", got.PosInParentTrace, got.Subtraces, wantPosition)
			}
			if got.Value == nil || got.Value.ToInt().Cmp(big.NewInt(42)) != 0 {
				t.Errorf("SELFDESTRUCT value = %v, want 42", got.Value)
			}

			// Reverting an ancestor changes classification and errors, but must
			// preserve every trace's identity, hierarchy, path and other fields.
			baseline := traceSelfDestruct(tc.depth, tc.precedingCall, -1)
			all := append(append([]ptypes.Trace{}, bf.Traces...), bf.ErrorTraces...)
			if len(all) != len(baseline.Traces) {
				t.Fatalf("trace count = %d, want %d", len(all), len(baseline.Traces))
			}
			byID := make(map[string]ptypes.Trace, len(all))
			for _, tr := range all {
				if _, exists := byID[tr.ID]; exists {
					t.Fatalf("duplicate trace ID %s", tr.ID)
				}
				tr.Error = ""
				byID[tr.ID] = tr
			}
			for _, want := range baseline.Traces {
				if got, ok := byID[want.ID]; !ok || !reflect.DeepEqual(got, want) {
					t.Errorf("trace %s changed beyond its error/classification: got %+v, want %+v", want.ID, got, want)
				}
			}
		})
	}
}

func TestParentFailurePreservesOwnRevertReason(t *testing.T) {
	tr := newTestCallTracer()
	tr.callstack = []callFrame{{
		Type: vm.CALL, To: testAddr(1), Error: "execution reverted",
		Calls: []callFrame{{
			Type: vm.CALL, To: testAddr(2), Error: "execution reverted", RevertReason: "original reason",
		}},
	}}
	tr.OnTxEnd(nil, nil)
	if got := findTraceByTo(t, tr.BlockFile.ErrorTraces, testAddr(2)).Error; got != "execution reverted: original reason" {
		t.Errorf("child error = %q, want own revert reason", got)
	}
}
