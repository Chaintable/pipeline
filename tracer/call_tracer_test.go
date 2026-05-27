package tracer

import (
	"math/big"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"
)

func addr(h string) common.Address { return common.HexToAddress(h) }

func addrPtr(a common.Address) *common.Address { return &a }

func makeLog(addrHex string, topicsHex []string, dataHex string) *types.Log {
	topics := make([]common.Hash, len(topicsHex))
	for i, h := range topicsHex {
		topics[i] = common.HexToHash(h)
	}
	return &types.Log{
		Address: common.HexToAddress(addrHex),
		Topics:  topics,
		Data:    common.FromHex(dataHex),
	}
}

func newDummyTx(nonce uint64) *types.Transaction {
	to := common.Address{}
	return types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		Gas:      21000,
		GasPrice: big.NewInt(1),
		To:       &to,
		Value:    big.NewInt(0),
	})
}

// buildSampleTree drives the callTracer through OnEnter/OnExit (v1.15 hooks
// API) to construct the following frame tree, with all sub-calls finalized
// into root.Calls:
//
//	root           trace_address = []
//	├── c          trace_address = [0]
//	│   ├── d      trace_address = [0, 0]
//	│   └── e      trace_address = [0, 1]
//	│       └── f  trace_address = [0, 1, 0]
//	└── g          trace_address = [1]
//
// Each frame's Input is set to its name so tests can identify which frame a
// log landed in without parsing addresses. Root is still on the callstack
// after the final OnExit(depth=0) because that path routes to captureEnd()
// which doesn't pop — matching the post-tx state expected by OnTxEnd.
func buildSampleTree(t *testing.T) *callTracer {
	t.Helper()
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(make(map[common.Address]struct{}), bf)
	gas := uint64(100000)
	zero := big.NewInt(0)

	ct.OnEnter(0, byte(vm.CALL), addr("0xA"), addr("0xB"), []byte("root"), gas, zero)
	ct.OnEnter(1, byte(vm.CALL), addr("0xB"), addr("0xC"), []byte("c"), gas, zero)
	ct.OnEnter(2, byte(vm.CALL), addr("0xC"), addr("0xD"), []byte("d"), gas, zero)
	ct.OnExit(2, []byte("d_out"), 100, nil, false)
	ct.OnEnter(2, byte(vm.STATICCALL), addr("0xC"), addr("0xE"), []byte("e"), gas, zero)
	ct.OnEnter(3, byte(vm.DELEGATECALL), addr("0xE"), addr("0xF"), []byte("f"), gas, zero)
	ct.OnExit(3, []byte("f_out"), 100, nil, false)
	ct.OnExit(2, []byte("e_out"), 200, nil, false)
	ct.OnExit(1, []byte("c_out"), 500, nil, false)
	ct.OnEnter(1, byte(vm.CALL), addr("0xB"), addr("0xG"), []byte("g"), gas, zero)
	ct.OnExit(1, []byte("g_out"), 100, nil, false)
	ct.OnExit(0, []byte("root_out"), 1000, nil, false)

	require.Len(t, ct.callstack, 1, "only root should remain on callstack")
	require.Len(t, ct.callstack[0].Calls, 2, "root has two direct children: c and g")
	require.Equal(t, "c", string(ct.callstack[0].Calls[0].Input))
	require.Equal(t, "g", string(ct.callstack[0].Calls[1].Input))
	require.Equal(t, "d", string(ct.callstack[0].Calls[0].Calls[0].Input))
	require.Equal(t, "e", string(ct.callstack[0].Calls[0].Calls[1].Input))
	require.Equal(t, "f", string(ct.callstack[0].Calls[0].Calls[1].Calls[0].Input))
	return ct
}

// TestCallTracerInTxLogIdx covers 13b55f1 (stamp per-tx InTxLogIdx on each
// Event). OnTxStart resets the counter; OnLog and InsertLog both stamp the
// current value into Event.InTxLogIdx and post-increment; a fresh OnTxStart
// resets again so (txID, InTxLogIdx) is unique per event within a tx.
func TestCallTracerInTxLogIdx(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(make(map[common.Address]struct{}), bf)

	tx := newDummyTx(0)
	ct.OnTxStart(&tracing.VMContext{}, tx, common.Address{})
	require.Equal(t, int64(0), ct.inTxLogIdx, "OnTxStart resets inTxLogIdx to 0")

	// A root frame so OnLog attaches the event to callstack[0].Logs instead of
	// falling back to BlockFile.Events (mantle compat path, irrelevant here).
	ct.OnEnter(0, byte(vm.CALL), common.Address{}, addr("0xfeed"), nil, 100000, big.NewInt(0))

	ct.OnLog(makeLog("0xaa", []string{"0xdead"}, "0x01"))
	require.Equal(t, int64(1), ct.inTxLogIdx)
	require.Equal(t, int64(0), ct.callstack[0].Logs[0].InTxLogIdx, "first event stamped with 0")

	ct.OnLog(makeLog("0xbb", []string{"0xbeef"}, "0x02"))
	require.Equal(t, int64(2), ct.inTxLogIdx)
	require.Equal(t, int64(1), ct.callstack[0].Logs[1].InTxLogIdx, "second event stamped with 1")

	// InsertLog also advances the counter — must stay coherent with OnLog so
	// (txID, InTxLogIdx) remains a unique key for every event in the tx.
	ct.InsertLog([]int64{}, 99, makeLog("0xcc", nil, "0x03"))
	require.Equal(t, int64(3), ct.inTxLogIdx)
	require.Equal(t, int64(2), ct.callstack[0].Logs[2].InTxLogIdx, "InsertLog stamps then advances")

	// Cross-tx boundary: a second OnTxStart must reset the counter.
	tx2 := newDummyTx(1)
	ct.OnTxStart(&tracing.VMContext{}, tx2, common.Address{})
	require.Equal(t, int64(0), ct.inTxLogIdx, "OnTxStart resets across tx boundary")
}

// TestRPCTracerSetTxHash covers fb86ee5. SetTxHash must (a) overwrite
// callTracer.txID so subsequent trace IDs / addTraceAndLog hashing use the
// override, (b) record TxHashOverride on the block context, (c) cause OnTxEnd
// to replace the built tx.ID with the override and clear TxHashOverride so
// the next tx doesn't inherit it.
func TestRPCTracerSetTxHash(t *testing.T) {
	rt := &RPCTracer{
		currentBlock: &RPCBlockContext{
			BlockHeader: &ptypes.Header{
				BaseFeePerGas: (*hexutil.Big)(big.NewInt(0)),
			},
			BlockFile: &ptypes.BlockFile{
				Txs: []ptypes.Transaction{},
			},
			ChangeContracts: make(map[common.Address]struct{}),
		},
	}

	from := addr("0xabcd00000000000000000000000000000000abcd")
	tx := newDummyTx(0)
	rt.OnTxStart(&tracing.VMContext{}, tx, from)
	require.NotNil(t, rt.callTracer, "OnTxStart allocates the callTracer")
	require.Equal(t, tx.Hash().Hex(), rt.callTracer.txID, "default txID is tx.Hash() before SetTxHash")

	overrideID := "0xdeadbeefcafebabefeedface0000000000000000000000000000000000000000"
	rt.SetTxHash(overrideID)
	require.Equal(t, overrideID, rt.callTracer.txID, "SetTxHash overwrites callTracer.txID")
	require.Equal(t, overrideID, rt.currentBlock.TxHashOverride, "SetTxHash records override on block ctx")

	// Minimal root frame so OnTxEnd has something to walk.
	rt.OnEnter(0, byte(vm.CALL), from, addr("0xfeed"), nil, 21000, big.NewInt(0))
	rt.OnExit(0, nil, 21000, nil, false)

	receipt := &types.Receipt{Status: types.ReceiptStatusSuccessful, GasUsed: 21000}
	rt.OnTxEnd(receipt, nil)

	require.Len(t, rt.currentBlock.BlockFile.Txs, 1)
	require.Equal(t, overrideID, rt.currentBlock.BlockFile.Txs[0].ID, "Tx.ID must be replaced by override")
	require.Empty(t, rt.currentBlock.TxHashOverride, "TxHashOverride must be cleared after OnTxEnd")
}

// TestInsertLog covers 5129d84. The deferred-flush path uses InsertLog to
// physically attach a log to a sub-frame whose OnExit has already finalized
// it into parent.Calls. Position must trust the caller's pre-OnLog snapshot,
// invalid traceAddress must drop silently, and InTxLogIdx must keep
// incrementing across InsertLog calls.
func TestInsertLog(t *testing.T) {
	t.Run("empty traceAddress inserts into root frame", func(t *testing.T) {
		ct := buildSampleTree(t)
		l := makeLog("0xaa", []string{"0xdeadbeef"}, "0x1234")
		ct.InsertLog([]int64{}, 7, l)

		require.Len(t, ct.callstack[0].Logs, 1, "root should hold the inserted log")
		require.Equal(t, int64(7), ct.callstack[0].Logs[0].Position, "Position must use caller-supplied value")
		require.Equal(t, "0x00000000000000000000000000000000000000aa", ct.callstack[0].Logs[0].Address)
	})

	t.Run("traceAddress [0] inserts into first sub-call (c)", func(t *testing.T) {
		ct := buildSampleTree(t)
		l := makeLog("0xbb", nil, "0x")
		ct.InsertLog([]int64{0}, 3, l)

		c := &ct.callstack[0].Calls[0]
		require.Len(t, c.Logs, 1)
		require.Equal(t, int64(3), c.Logs[0].Position)
		require.Empty(t, ct.callstack[0].Logs, "root.Logs must be untouched")
	})

	t.Run("traceAddress [0,1,0] inserts into deeply nested frame (f)", func(t *testing.T) {
		ct := buildSampleTree(t)
		l := makeLog("0xff", []string{"0xaaaa", "0xbbbb"}, "0xff00")
		ct.InsertLog([]int64{0, 1, 0}, 9, l)

		f := &ct.callstack[0].Calls[0].Calls[1].Calls[0]
		require.Len(t, f.Logs, 1)
		require.Equal(t, int64(9), f.Logs[0].Position)
		require.Empty(t, ct.callstack[0].Logs)
		require.Empty(t, ct.callstack[0].Calls[0].Logs)
		require.Empty(t, ct.callstack[0].Calls[0].Calls[1].Logs)
	})

	t.Run("invalid traceAddress drops silently", func(t *testing.T) {
		ct := buildSampleTree(t)
		l := makeLog("0xee", nil, "0x")
		ct.InsertLog([]int64{99}, 0, l) // out-of-bounds at root
		require.Empty(t, ct.callstack[0].Logs)
		require.Empty(t, ct.callstack[0].Calls[0].Logs)
		ct.InsertLog([]int64{0, 99}, 0, l) // out-of-bounds at depth 1
		require.Empty(t, ct.callstack[0].Calls[0].Logs)
		ct.InsertLog([]int64{-1}, 0, l) // negative index
		require.Empty(t, ct.callstack[0].Logs)
	})

	t.Run("InTxLogIdx increments across frames in call order", func(t *testing.T) {
		ct := buildSampleTree(t)
		ct.inTxLogIdx = 0
		ct.InsertLog([]int64{0, 0}, 0, makeLog("0x11", nil, "0x"))
		ct.InsertLog([]int64{}, 5, makeLog("0x22", nil, "0x"))
		ct.InsertLog([]int64{1}, 0, makeLog("0x33", nil, "0x"))

		require.Equal(t, int64(0), ct.callstack[0].Calls[0].Calls[0].Logs[0].InTxLogIdx)
		require.Equal(t, int64(1), ct.callstack[0].Logs[0].InTxLogIdx)
		require.Equal(t, int64(2), ct.callstack[0].Calls[1].Logs[0].InTxLogIdx)
		require.Equal(t, int64(3), ct.inTxLogIdx)
	})

	t.Run("Position is caller-supplied, never recomputed from frame state", func(t *testing.T) {
		ct := buildSampleTree(t)
		// c frame has Calls=[d, e] (len 2) + Logs=[] (len 0). A naive
		// recomputation would give Position=2; we pass 99 to prove the API
		// trusts the caller's pre-OnLog snapshot.
		ct.InsertLog([]int64{0}, 99, makeLog("0xab", nil, "0x"))
		require.Equal(t, int64(99), ct.callstack[0].Calls[0].Logs[0].Position)
	})

	t.Run("interrupted tracer skips insertion", func(t *testing.T) {
		ct := buildSampleTree(t)
		ct.Stop(nil)
		ct.InsertLog([]int64{}, 0, makeLog("0xcc", nil, "0x"))
		require.Empty(t, ct.callstack[0].Logs)
	})

	t.Run("topics split into selector + remaining", func(t *testing.T) {
		ct := buildSampleTree(t)
		l := makeLog("0xdd", []string{"0x1111", "0x2222", "0x3333"}, "0x")
		ct.InsertLog([]int64{}, 0, l)

		ev := ct.callstack[0].Logs[0]
		require.Equal(t, common.HexToHash("0x1111").Hex(), ev.Selector)
		require.Equal(t, []string{common.HexToHash("0x2222").Hex(), common.HexToHash("0x3333").Hex()}, ev.Topics)
	})

	t.Run("empty topics produce empty selector", func(t *testing.T) {
		ct := buildSampleTree(t)
		ct.InsertLog([]int64{}, 0, makeLog("0xdd", nil, "0x"))

		ev := ct.callstack[0].Logs[0]
		require.Equal(t, "", ev.Selector)
		require.Empty(t, ev.Topics)
	})

	t.Run("empty callstack drops insertion (safety net)", func(t *testing.T) {
		ct := newCallTracerRaw(make(map[common.Address]struct{}), &ptypes.BlockFile{})
		// no OnEnter → callstack is empty
		require.NotPanics(t, func() {
			ct.InsertLog([]int64{}, 0, makeLog("0xff", nil, "0x"))
		})
	})
}

// TestSetPendingLogsOnTopParent covers 3d56e5b. Without the pending-logs
// hint, PosInParentTrace would compute from len(parent.Calls) +
// len(parent.Logs) only; with the hint, it adds the buffered (not-yet-
// inserted) log count, so sub-frames take positions interleaved with the
// buffered logs. The hint is consumed (used + reset to 0) by each OnExit so
// it doesn't leak forward.
func TestSetPendingLogsOnTopParent(t *testing.T) {
	t.Run("OnExit adds pending count to PosInParentTrace and resets", func(t *testing.T) {
		bf := &ptypes.BlockFile{}
		ct := newCallTracerRaw(make(map[common.Address]struct{}), bf)
		gas := uint64(100000)
		zero := big.NewInt(0)

		ct.OnEnter(0, byte(vm.CALL), addr("0xA"), addr("0xB"), []byte("root"), gas, zero)
		// Caller (iotex buffered tracer) signals 1 log was emitted on root
		// before the sub-call; the log itself is sitting in caller's pending
		// buffer (will be InsertLog'd later at pos=0).
		ct.OnEnter(1, byte(vm.CALL), addr("0xB"), addr("0xC"), []byte("c0"), gas, zero)
		ct.SetPendingLogsOnTopParent(1)
		ct.OnExit(1, nil, 100, nil, false)

		// Without the hint, PosInParentTrace would be 0+0=0. With pending=1,
		// it becomes 1 — making room for the buffered log at pos=0.
		require.Equal(t, 1, ct.callstack[0].Calls[0].PosInParentTrace, "sub takes pos after the buffered log")
		require.Equal(t, 0, ct.pendingLogsOnTopParent, "hint is consumed by OnExit")

		// Second sub-call without Set: pending=0, uses raw
		// len(Calls)+len(Logs)=1+0=1 (no contamination from the previous Set).
		ct.OnEnter(1, byte(vm.CALL), addr("0xB"), addr("0xD"), []byte("c1"), gas, zero)
		ct.OnExit(1, nil, 100, nil, false)
		require.Equal(t, 1, ct.callstack[0].Calls[1].PosInParentTrace, "no pending → use len(Calls)+len(Logs)")

		ct.OnExit(0, nil, 1000, nil, false)
	})

	t.Run("two consecutive Set calls is last-write-wins", func(t *testing.T) {
		bf := &ptypes.BlockFile{}
		ct := newCallTracerRaw(make(map[common.Address]struct{}), bf)
		ct.OnEnter(0, byte(vm.CALL), addr("0xA"), addr("0xB"), nil, 0, big.NewInt(0))
		ct.OnEnter(1, byte(vm.CALL), addr("0xB"), addr("0xC"), nil, 0, big.NewInt(0))

		ct.SetPendingLogsOnTopParent(5)
		ct.SetPendingLogsOnTopParent(2)
		ct.OnExit(1, nil, 0, nil, false)
		require.Equal(t, 2, ct.callstack[0].Calls[0].PosInParentTrace, "last Set wins")
	})
}

// TestOrphanOnExitResetsPending covers e76a1ce. When OnExit takes the
// size<=1 early-return path (depth!=0 + nothing meaningful to pop), it must
// still clear the caller-set pendingLogsOnTopParent. Otherwise a misbehaving
// caller path that pairs Set+Exit when the stack is empty would corrupt the
// PosInParentTrace of a totally unrelated later sub-frame.
//
// Note on depth choice: must use depth!=0 (here depth=1) — depth=0 routes to
// captureEnd() which does NOT touch pendingLogsOnTopParent. Only the
// depth>=1 + size<=1 early-return path performs the reset.
func TestOrphanOnExitResetsPending(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(make(map[common.Address]struct{}), bf)

	ct.SetPendingLogsOnTopParent(42)
	require.Equal(t, 42, ct.pendingLogsOnTopParent)
	ct.OnExit(1, nil, 0, nil, false)
	require.Equal(t, 0, ct.pendingLogsOnTopParent, "orphan early-return must clear pending")

	// Now run a normal tx. The leftover 42 must NOT contaminate the new
	// sub-call's PosInParentTrace.
	ct.OnEnter(0, byte(vm.CALL), addr("0xA"), addr("0xB"), nil, 0, big.NewInt(0))
	ct.OnEnter(1, byte(vm.CALL), addr("0xB"), addr("0xC"), nil, 0, big.NewInt(0))
	ct.OnExit(1, nil, 0, nil, false)
	require.Equal(t, 0, ct.callstack[0].Calls[0].PosInParentTrace,
		"leftover pendingLogsOnTopParent=42 must have been cleared by orphan exit")
}

// TestSELFDESTRUCTCallType covers a824e0b. Before the fix ToTrace only set
// CallCreateType="suicide" for SELFDESTRUCT frames, leaving CallType=""
// which downstream leafage treats as a no-op trace. After the fix both
// fields populate so leafage can identify the trace as a SELFDESTRUCT.
func TestSELFDESTRUCTCallType(t *testing.T) {
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(make(map[common.Address]struct{}), bf)

	build := func(op vm.OpCode) *callFrame {
		return &callFrame{
			Type:  op,
			From:  addr("0xaaaa"),
			To:    addrPtr(addr("0xbbbb")),
			Gas:   5000,
			Value: big.NewInt(0),
		}
	}

	sd := ct.ToTrace(build(vm.SELFDESTRUCT), []int64{0})
	require.Equal(t, "suicide", sd.CallCreateType)
	require.Equal(t, "selfdestruct", sd.CallType, "SELFDESTRUCT must set CallType (a824e0b)")

	cr := ct.ToTrace(build(vm.CREATE), []int64{1})
	require.Equal(t, "create", cr.CallCreateType)
	require.Equal(t, "", cr.CallType, "CREATE leaves CallType empty (control)")

	cl := ct.ToTrace(build(vm.CALL), []int64{2})
	require.Equal(t, "call", cl.CallCreateType)
	require.Equal(t, "call", cl.CallType, "CALL sets both fields")
}
