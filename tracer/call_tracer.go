package tracer

// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

const parentCallFailedError = "parent call failed"

type callFrame struct {
	Type         vm.OpCode       `json:"-"`
	From         common.Address  `json:"from"`
	Gas          uint64          `json:"gas"`
	GasUsed      uint64          `json:"gasUsed"`
	To           *common.Address `json:"to,omitempty" rlp:"optional"`
	Input        []byte          `json:"input" rlp:"optional"`
	Output       []byte          `json:"output,omitempty" rlp:"optional"`
	Error        string          `json:"error,omitempty" rlp:"optional"`
	ParentFailed bool            `json:"-"` // Indicates if the parent call failed
	Precompile   bool            `json:"-"`
	Calls        []callFrame     `json:"calls,omitempty" rlp:"optional"`
	Logs         []ptypes.Event  `json:"logs,omitempty" rlp:"optional"`

	PosInParentTrace  int    `json:"pos_in_parent_trace"`
	ParentTraceID     string `json:"parent_trace_id"`
	TraceID           string `json:"trace_id"`
	StorageChange     bool   `json:"storageChange"`
	SelfStorageChange bool   `json:"self_storage_change"`

	// Placed at end on purpose. The RLP will be decoded to 0 instead of
	// nil if there are non-empty elements after in the struct.
	Value *big.Int `json:"value,omitempty" rlp:"optional"`
}

func (f callFrame) TypeString() string {
	return f.Type.String()
}

func (f callFrame) failed() bool {
	return len(f.Error) > 0
}

func (f *callFrame) processOutput(output []byte, err error) {
	f.Output = common.CopyBytes(output)
	if err != nil {
		f.Error = formatTraceError(err, f.Precompile)
	}
}

func formatTraceError(err error, precompile bool) string {
	switch {
	case errors.Is(err, vm.ErrExecutionReverted):
		return "Reverted"
	case errors.Is(err, vm.ErrOutOfGas),
		errors.Is(err, vm.ErrCodeStoreOutOfGas),
		errors.Is(err, vm.ErrGasUintOverflow):
		return "Out of gas"
	case errors.Is(err, vm.ErrInsufficientBalance):
		return "Insufficient balance for transfer"
	case errors.Is(err, vm.ErrInvalidJump):
		return "Bad jump destination"
	case errors.Is(err, vm.ErrDepth):
		return "CallTooDeep"
	case errors.Is(err, vm.ErrContractAddressCollision):
		return "CreateCollision"
	case errors.Is(err, vm.ErrWriteProtection):
		return "StateChangeDuringStaticCall"
	case errors.Is(err, vm.ErrReturnDataOutOfBounds):
		return "OutOfOffset"
	case errors.Is(err, vm.ErrNonceUintOverflow):
		return "NonceOverflow"
	case errors.Is(err, vm.ErrMaxCodeSizeExceeded):
		return "CreateContractSizeLimit"
	case errors.Is(err, vm.ErrMaxInitCodeSizeExceeded):
		return "CreateInitCodeSizeLimit"
	case errors.Is(err, vm.ErrInvalidCode):
		return "CreateContractStartingWithEF"
	}
	var invalidOpcode *vm.ErrInvalidOpCode
	if errors.As(err, &invalidOpcode) {
		return "Bad instruction"
	}
	var stackOverflow *vm.ErrStackOverflow
	if errors.As(err, &stackOverflow) {
		return "Out of stack"
	}
	var stackUnderflow *vm.ErrStackUnderflow
	if errors.As(err, &stackUnderflow) {
		return "StackUnderflow"
	}
	if precompile {
		return "Built-in failed"
	}
	return err.Error()
}

type callTracer struct {
	callstack           []callFrame
	gasLimit            uint64
	depth               int
	interrupt           atomic.Bool // Atomic flag to signal execution interruption
	reason              error       // Textual reason for the interruption
	precompileAddresses map[common.Address]struct{}

	txID string

	ChangeContracts map[common.Address]struct{}
	BlockFile       *ptypes.BlockFile
}

func newCallTracerRaw(ChangeContracts map[common.Address]struct{}, BlockFile *ptypes.BlockFile) *callTracer {
	t := &callTracer{
		callstack:           make([]callFrame, 0, 1),
		precompileAddresses: make(map[common.Address]struct{}),
		ChangeContracts:     ChangeContracts,
		BlockFile:           BlockFile,
	}
	return t
}

func (t *callTracer) isPrecompile(address common.Address) bool {
	_, ok := t.precompileAddresses[address]
	return ok
}

func (t *callTracer) ToTrace(f *callFrame, traceAddress []int64) ptypes.Trace {
	CallCreateType := ""
	CallType := ""
	switch f.Type {
	case vm.CREATE:
		CallCreateType = strings.ToLower(vm.CREATE.String())
	case vm.CREATE2:
		CallCreateType = strings.ToLower(vm.CREATE2.String())
	case vm.SELFDESTRUCT:
		CallCreateType = "suicide"
	case vm.CALL, vm.STATICCALL, vm.CALLCODE, vm.DELEGATECALL:
		CallCreateType = strings.ToLower(vm.CALL.String())
		CallType = strings.ToLower(f.Type.String())
	default:
		CallCreateType = "empty"
	}
	to := common.Address{}
	if f.To != nil {
		to = *f.To
	}
	value := big.NewInt(0)
	if f.Value != nil {
		value = f.Value
	}
	err := ""
	if f.failed() {
		err = f.Error
	} else if f.ParentFailed {
		err = parentCallFailedError
	}
	return ptypes.Trace{
		ID:                f.TraceID,
		From:              strings.ToLower(f.From.Hex()),
		Gas:               big.NewInt(int64(f.Gas)),
		Input:             (hexutil.Bytes)(f.Input),
		To:                strings.ToLower(to.Hex()),
		Value:             (*hexutil.Big)(value),
		GasUsed:           big.NewInt(int64(f.GasUsed)),
		Output:            (hexutil.Bytes)(f.Output),
		CallCreateType:    CallCreateType,
		CallType:          CallType,
		TxID:              t.txID,
		ParentTraceID:     f.ParentTraceID,
		PosInParentTrace:  int64(f.PosInParentTrace),
		SelfStorageChange: f.SelfStorageChange,
		StorageChange:     f.StorageChange,
		Subtraces:         int64(len(f.Calls)),
		TraceAddress:      traceAddress,
		Error:             err,
	}
}

// vm.EVMLogger interface implementation

func (t *callTracer) CaptureTxStart(gasLimit uint64) {
	t.gasLimit = gasLimit
}

func (t *callTracer) CaptureTxEnd(restGas uint64) {
}

func (t *callTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.depth = 0
	if t.interrupt.Load() {
		return
	}
	if env != nil {
		rules := env.ChainConfig().Rules(env.Context.BlockNumber, env.Context.Random != nil, env.Context.Time)
		for _, address := range vm.ActivePrecompiles(rules) {
			t.precompileAddresses[address] = struct{}{}
		}
	}

	toCopy := to
	typ := vm.CALL
	if create {
		typ = vm.CREATE
	}
	call := callFrame{
		Type:       typ,
		From:       from,
		To:         &toCopy,
		Input:      common.CopyBytes(input),
		Gas:        t.gasLimit,
		Value:      value,
		Precompile: t.isPrecompile(to),
	}
	t.callstack = append(t.callstack, call)
}

func (t *callTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	if len(t.callstack) != 1 {
		return
	}
	t.callstack[0].GasUsed = gasUsed
	t.callstack[0].processOutput(output, err)
}

func (t *callTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	t.depth++
	if t.interrupt.Load() {
		return
	}

	toCopy := to
	call := callFrame{
		Type:       typ,
		From:       from,
		To:         &toCopy,
		Input:      common.CopyBytes(input),
		Gas:        gas,
		Value:      value,
		Precompile: t.isPrecompile(to),
	}
	t.callstack = append(t.callstack, call)
}

func (t *callTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
	t.depth--

	size := len(t.callstack)
	if size <= 1 {
		return
	}

	call := t.callstack[size-1]
	t.callstack = t.callstack[:size-1]
	size -= 1

	call.GasUsed = gasUsed
	call.processOutput(output, err)
	call.PosInParentTrace = len(t.callstack[size-1].Calls) + len(t.callstack[size-1].Logs)
	t.callstack[size-1].Calls = append(t.callstack[size-1].Calls, call)
}

// CaptureEarlyExit records CALL/CREATE attempts rejected before the legacy
// EVMLogger CaptureEnter hook is reached.
func (t *callTracer) CaptureEarlyExit(depth int, typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int, gasUsed uint64, err error) {
	if t.interrupt.Load() {
		return
	}
	toCopy := to
	call := callFrame{
		Type:       typ,
		From:       from,
		To:         &toCopy,
		Input:      common.CopyBytes(input),
		Gas:        gas,
		GasUsed:    gasUsed,
		Value:      value,
		Precompile: t.isPrecompile(to),
	}
	call.processOutput(nil, err)
	if depth == 0 || len(t.callstack) == 0 {
		t.callstack = append(t.callstack, call)
		return
	}
	parent := &t.callstack[len(t.callstack)-1]
	call.PosInParentTrace = len(parent.Calls) + len(parent.Logs)
	parent.Calls = append(parent.Calls, call)
}

func (t *callTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	if len(t.callstack) == 0 {
		return
	}
	if op == vm.SSTORE {
		t.callstack[len(t.callstack)-1].SelfStorageChange = true
		t.callstack[len(t.callstack)-1].StorageChange = true
	}
}

func (t *callTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
}

// Custom methods for RPCTracer

func (t *callTracer) OnTxStart(tx *types.Transaction, from common.Address) {
	t.gasLimit = tx.Gas()
	t.txID = tx.Hash().Hex()
}

func (t *callTracer) OnTxEnd(receipt *types.Receipt, err error) {
	if err != nil {
		return
	}
	if len(t.callstack) == 0 {
		return
	}
	setParentFailed(&t.callstack[0], false)
	setStorageChange(&t.callstack[0], t.ChangeContracts)
	if len(t.callstack) == 1 {
		topCall := &t.callstack[0]
		topCall.TraceID = util.ToHash([]string{t.txID, "", "0"})
		if topCall.failed() {
			t.BlockFile.ErrorTraces = append(t.BlockFile.ErrorTraces, t.ToTrace(topCall, []int64{}))
		} else {
			t.BlockFile.Traces = append(t.BlockFile.Traces, t.ToTrace(topCall, []int64{}))
		}
		t.addTraceAndLog(topCall, []int64{})
	}
}

func (t *callTracer) OnLog(log *types.Log) {
	// Skip if tracing was interrupted
	if t.interrupt.Load() {
		return
	}
	topics := make([]string, len(log.Topics))
	for i, topic := range log.Topics {
		topics[i] = topic.Hex()
	}
	var selector string
	var remainingTopics []string

	if len(topics) > 0 {
		selector = topics[0]
		remainingTopics = topics[1:]
	}

	var position int64
	if len(t.callstack) > 0 {
		position = int64(len(t.callstack[len(t.callstack)-1].Calls) + len(t.callstack[len(t.callstack)-1].Logs))
	} else {
		// 对于某些链(例如mantle),这个event发生在所有call之前,直接置为0并添加到最终的event中
		position = 0
	}

	l := ptypes.Event{
		Address:  strings.ToLower(log.Address.Hex()),
		Selector: selector,
		Topics:   remainingTopics,
		Data:     log.Data,
		Position: position,
		LogIndex: int64(log.Index),
	}

	if len(t.callstack) > 0 {
		t.callstack[len(t.callstack)-1].Logs = append(t.callstack[len(t.callstack)-1].Logs, l)
	} else {
		t.BlockFile.Events = append(t.BlockFile.Events, l)
	}
}

func (t *callTracer) GetResult() (json.RawMessage, error) {
	return nil, nil
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *callTracer) Stop(err error) {
	t.reason = err
	t.interrupt.Store(true)
}

// setParentFailed recursively sets the ParentFailed flag for the call frame and all its subcalls.
func setParentFailed(cf *callFrame, parentFailed bool) {
	failed := cf.failed() || parentFailed
	for i := range cf.Calls {
		cf.Calls[i].ParentFailed = failed
		setParentFailed(&cf.Calls[i], failed)
	}
}

func setStorageChange(cf *callFrame, ChangeContracts map[common.Address]struct{}) {
	if cf.To != nil && cf.SelfStorageChange {
		if cf.Type == vm.DELEGATECALL {
			ChangeContracts[cf.From] = struct{}{}
		} else {
			ChangeContracts[*cf.To] = struct{}{}
		}
	}
	subCallStorageChange := false
	for i := range cf.Calls {
		setStorageChange(&cf.Calls[i], ChangeContracts)
		if cf.Calls[i].StorageChange && !cf.Calls[i].failed() {
			subCallStorageChange = true
		}
	}
	if subCallStorageChange {
		cf.StorageChange = true
	}
}

func (t *callTracer) addTraceAndLog(cf *callFrame, traceAddress []int64) {
	callIndex, logIndex := 0, 0
	for callIndex < len(cf.Calls) || logIndex < len(cf.Logs) {
		if logIndex >= len(cf.Logs) || (callIndex < len(cf.Calls) && cf.Calls[callIndex].PosInParentTrace < int(cf.Logs[logIndex].Position)) {
			child := &cf.Calls[callIndex]
			child.ParentTraceID = cf.TraceID
			child.TraceID = util.ToHash([]string{t.txID, cf.TraceID, fmt.Sprintf("%d", child.PosInParentTrace)})
			childAddress := childTraceAddress(traceAddress, int64(callIndex))
			if child.failed() || child.ParentFailed {
				t.BlockFile.ErrorTraces = append(t.BlockFile.ErrorTraces, t.ToTrace(child, childAddress))
			} else {
				t.BlockFile.Traces = append(t.BlockFile.Traces, t.ToTrace(child, childAddress))
			}
			t.addTraceAndLog(child, childAddress)
			callIndex++
			continue
		}

		event := &cf.Logs[logIndex]
		event.ParentTraceID = cf.TraceID
		event.ID = util.ToHash([]string{event.ParentTraceID, fmt.Sprintf("%d", event.Position)})
		if cf.failed() || cf.ParentFailed {
			event.LogIndex = 0
			t.BlockFile.ErrorEvents = append(t.BlockFile.ErrorEvents, *event)
		} else {
			t.BlockFile.Events = append(t.BlockFile.Events, *event)
		}
		logIndex++
	}
}

func childTraceAddress(a []int64, i int64) []int64 {
	child := make([]int64, 0, len(a)+1)
	child = append(child, a...)
	child = append(child, i)
	return child
}
