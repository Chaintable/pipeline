package tracer

import (
	"math/big"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"
)

// buildSampleTree constructs a callTracer whose callstack has only root, with
// the following frame tree fully finalized via CaptureExit into root.Calls:
//
//	root           trace_address = []
//	├── c          trace_address = [0]
//	│   ├── d      trace_address = [0, 0]
//	│   └── e      trace_address = [0, 1]
//	│       └── f  trace_address = [0, 1, 0]
//	└── g          trace_address = [1]
//
// Each frame's Input field is set to its name (e.g. "root", "c", "f") so tests
// can assert which frame a log landed in without parsing addresses.
func buildSampleTree(t *testing.T) *callTracer {
	t.Helper()
	bf := &ptypes.BlockFile{}
	ct := newCallTracerRaw(make(map[common.Address]struct{}), bf)
	addr := func(hex string) common.Address { return common.HexToAddress(hex) }
	gas := uint64(100000)
	zero := big.NewInt(0)

	ct.CaptureStart(nil, addr("0xA"), addr("0xB"), false, []byte("root"), gas, zero)
	ct.CaptureEnter(vm.CALL, addr("0xB"), addr("0xC"), []byte("c"), gas, zero)
	ct.CaptureEnter(vm.CALL, addr("0xC"), addr("0xD"), []byte("d"), gas, zero)
	ct.CaptureExit([]byte("d_out"), 100, nil)
	ct.CaptureEnter(vm.STATICCALL, addr("0xC"), addr("0xE"), []byte("e"), gas, zero)
	ct.CaptureEnter(vm.DELEGATECALL, addr("0xE"), addr("0xF"), []byte("f"), gas, zero)
	ct.CaptureExit([]byte("f_out"), 100, nil)
	ct.CaptureExit([]byte("e_out"), 200, nil)
	ct.CaptureExit([]byte("c_out"), 500, nil)
	ct.CaptureEnter(vm.CALL, addr("0xB"), addr("0xG"), []byte("g"), gas, zero)
	ct.CaptureExit([]byte("g_out"), 100, nil)
	ct.CaptureEnd([]byte("root_out"), 1000, nil)

	require.Len(t, ct.callstack, 1, "only root should remain on callstack")
	require.Len(t, ct.callstack[0].Calls, 2, "root has two direct children: c and g")
	require.Equal(t, "c", string(ct.callstack[0].Calls[0].Input))
	require.Equal(t, "g", string(ct.callstack[0].Calls[1].Input))
	require.Equal(t, "d", string(ct.callstack[0].Calls[0].Calls[0].Input))
	require.Equal(t, "e", string(ct.callstack[0].Calls[0].Calls[1].Input))
	require.Equal(t, "f", string(ct.callstack[0].Calls[0].Calls[1].Calls[0].Input))
	return ct
}

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

func TestCallTracer_InsertLog(t *testing.T) {
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
		// out-of-bounds at root
		ct.InsertLog([]int64{99}, 0, l)
		require.Empty(t, ct.callstack[0].Logs)
		require.Empty(t, ct.callstack[0].Calls[0].Logs)
		// out-of-bounds at depth 1
		ct.InsertLog([]int64{0, 99}, 0, l)
		require.Empty(t, ct.callstack[0].Calls[0].Logs)
		// negative index
		ct.InsertLog([]int64{-1}, 0, l)
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
		// c frame has Calls=[d, e] (len 2) + Logs=[] (len 0). A naive recomputation
		// would give Position=2; we pass 99 to prove the API trusts the caller.
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
		// no CaptureStart → callstack is empty
		require.NotPanics(t, func() {
			ct.InsertLog([]int64{}, 0, makeLog("0xff", nil, "0x"))
		})
	})
}
