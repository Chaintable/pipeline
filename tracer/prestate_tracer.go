package tracer

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/morph-l2/go-ethereum/common"
	"github.com/morph-l2/go-ethereum/core/tracing"
	"github.com/morph-l2/go-ethereum/core/types"
	"github.com/morph-l2/go-ethereum/core/vm"
	"github.com/morph-l2/go-ethereum/crypto"
	"github.com/morph-l2/go-ethereum/log"
	"github.com/holiman/uint256"
)

type stateMap = map[common.Address]*account

type account struct {
	Balance *big.Int                    `json:"balance,omitempty"`
	Code    []byte                      `json:"code,omitempty"`
	Nonce   uint64                      `json:"nonce,omitempty"`
	Storage map[common.Hash]common.Hash `json:"storage,omitempty"`
	empty   bool
}

func (a *account) exists() bool {
	return a.Nonce > 0 || len(a.Code) > 0 || len(a.Storage) > 0 || (a.Balance != nil && a.Balance.Sign() != 0)
}

type prestateTracer struct {
	env       *tracing.VMContext
	pre       stateMap
	post      stateMap
	to        common.Address
	config    prestateTracerConfig
	interrupt atomic.Bool // Atomic flag to signal execution interruption
	reason    error       // Textual reason for the interruption
	created   map[common.Address]bool
	deleted   map[common.Address]bool
}

type prestateTracerConfig struct {
	DiffMode bool `json:"diffMode"` // If true, this tracer will return state modifications
}

func newPrestateTracer(config *prestateTracerConfig) *prestateTracer {
	t := &prestateTracer{
		pre:     stateMap{},
		post:    stateMap{},
		config:  *config,
		created: make(map[common.Address]bool),
		deleted: make(map[common.Address]bool),
	}
	return t
}

// OnOpcode implements the EVMLogger interface to trace a single step of VM execution.
func (t *prestateTracer) OnOpcode(pc uint64, opcode byte, gas, cost uint64, scope tracing.OpContext, rData []byte, depth int, err error) {
	if err != nil {
		return
	}
	// Skip if tracing was interrupted
	if t.interrupt.Load() {
		return
	}
	op := vm.OpCode(opcode)
	stackData := scope.StackData()
	stackLen := len(stackData)
	caller := scope.Address()
	switch {
	case stackLen >= 1 && (op == vm.SLOAD || op == vm.SSTORE):
		slot := common.Hash(stackData[stackLen-1].Bytes32())
		t.lookupStorage(caller, slot)
	case stackLen >= 1 && (op == vm.EXTCODECOPY || op == vm.EXTCODEHASH || op == vm.EXTCODESIZE || op == vm.BALANCE || op == vm.SELFDESTRUCT):
		addr := common.Address(stackData[stackLen-1].Bytes20())
		t.lookupAccount(addr)
		if op == vm.SELFDESTRUCT {
			t.deleted[caller] = true
		}
	case stackLen >= 5 && (op == vm.DELEGATECALL || op == vm.CALL || op == vm.STATICCALL || op == vm.CALLCODE):
		addr := common.Address(stackData[stackLen-2].Bytes20())
		t.lookupAccount(addr)
	case op == vm.CREATE:
		nonce := t.env.StateDB.GetNonce(caller)
		addr := crypto.CreateAddress(caller, nonce)
		t.lookupAccount(addr)
		t.created[addr] = true
	case stackLen >= 4 && op == vm.CREATE2:
		offset := stackData[stackLen-2]
		size := stackData[stackLen-3]
		init, err := GetMemoryCopyPadded(scope.MemoryData(), int64(offset.Uint64()), int64(size.Uint64()))
		if err != nil {
			log.Warn("failed to copy CREATE2 input", "err", err, "tracer", "prestateTracer", "offset", offset, "size", size)
			return
		}
		inithash := crypto.Keccak256(init)
		salt := stackData[stackLen-4]
		addr := crypto.CreateAddress2(caller, salt.Bytes32(), inithash)
		t.lookupAccount(addr)
		t.created[addr] = true
	}
}

func (t *prestateTracer) OnSystemCallStartHookV2(vm *tracing.VMContext) {
	t.env = vm
}

func (t *prestateTracer) OnTxStart(env *tracing.VMContext, tx *types.Transaction, from common.Address) {
	t.env = env
	if tx.To() == nil {
		t.to = crypto.CreateAddress(from, env.StateDB.GetNonce(from))
		t.created[t.to] = true
	} else {
		t.to = *tx.To()
	}

	t.lookupAccount(from)
	t.lookupAccount(t.to)
	t.lookupAccount(env.Coinbase)

	// Add accounts with authorizations to the prestate before they get applied.
	for _, auth := range tx.SetCodeAuthorizations() {
		addr, err := auth.Authority()
		if err != nil {
			continue
		}
		t.lookupAccount(addr)
	}
}

func (t *prestateTracer) OnTxEnd(receipt *types.Receipt, err error) {
	if err != nil {
		return
	}
	if t.config.DiffMode {
		t.processDiffState()
	}
	// the new created contracts' prestate were empty, so delete them
	for a := range t.created {
		// the created contract maybe exists in statedb before the creating tx
		if s := t.pre[a]; s != nil && s.empty {
			delete(t.pre, a)
		}
	}
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *prestateTracer) Stop(err error) {
	t.reason = err
	t.interrupt.Store(true)
}

func (t *prestateTracer) processDiffState() {
	for addr, state := range t.pre {
		// The deleted account's state is pruned from `post` but kept in `pre`
		if _, ok := t.deleted[addr]; ok {
			continue
		}
		modified := false
		postAccount := &account{Storage: make(map[common.Hash]common.Hash)}
		newBalance := t.env.StateDB.GetBalance(addr)
		newNonce := t.env.StateDB.GetNonce(addr)
		newCode := t.env.StateDB.GetCode(addr)

		if newBalance.Cmp(t.pre[addr].Balance) != 0 {
			modified = true
			postAccount.Balance = newBalance
		}
		if newNonce != t.pre[addr].Nonce {
			modified = true
			postAccount.Nonce = newNonce
		}
		if !bytes.Equal(newCode, t.pre[addr].Code) {
			modified = true
			postAccount.Code = newCode
		}

		for key, val := range state.Storage {
			// don't include the empty slot
			if val == (common.Hash{}) {
				delete(t.pre[addr].Storage, key)
			}

			newVal := t.env.StateDB.GetState(addr, key)
			if val == newVal {
				// Omit unchanged slots
				delete(t.pre[addr].Storage, key)
			} else {
				modified = true
				if newVal != (common.Hash{}) {
					postAccount.Storage[key] = newVal
				}
			}
		}

		if modified {
			t.post[addr] = postAccount
		} else {
			// if state is not modified, then no need to include into the pre state
			delete(t.pre, addr)
		}
	}
}

// lookupAccount fetches details of an account and adds it to the prestate
// if it doesn't exist there.
func (t *prestateTracer) lookupAccount(addr common.Address) {
	if _, ok := t.pre[addr]; ok {
		return
	}

	acc := &account{
		Balance: t.env.StateDB.GetBalance(addr),
		Nonce:   t.env.StateDB.GetNonce(addr),
		Code:    t.env.StateDB.GetCode(addr),
		Storage: make(map[common.Hash]common.Hash),
	}
	if !acc.exists() {
		acc.empty = true
	}
	t.pre[addr] = acc
}

// lookupStorage fetches the requested storage slot and adds
// it to the prestate of the given contract. It assumes `lookupAccount`
// has been performed on the contract before.
func (t *prestateTracer) lookupStorage(addr common.Address, key common.Hash) {
	if _, ok := t.pre[addr].Storage[key]; ok {
		return
	}
	t.pre[addr].Storage[key] = t.env.StateDB.GetState(addr, key)
}

func (t *prestateTracer) OnBlockDBStart(db tracing.StateDB) {
	t.env = &tracing.VMContext{
		StateDB: db,
	}
}

func (t *prestateTracer) GetStateDiff(originRoot common.Hash, root common.Hash) *ptypes.BlockStorageDiff {
	t.processDiffState()
	stateDiff := &ptypes.BlockStorageDiff{}
	if originRoot == (common.Hash{}) {
		originRoot = types.EmptyRootHash
	}
	if root == (common.Hash{}) {
		root = types.EmptyRootHash
	}
	stateDiff.Hash = root
	stateDiff.ParentHash = originRoot

	for addr, newAccount := range t.post {
		oldAccount, exists := t.pre[addr]
		if !exists {
			// If the account does not exist in prestate, it is a new create account
			oldAccount = &account{
				Balance: big.NewInt(0),
				Nonce:   0,
				empty:   true,
			}
		}

		// only storage changes
		if newAccount.Nonce == 0 && len(newAccount.Code) == 0 && newAccount.Balance == nil {
			continue
		}
		if newAccount.Balance != nil || newAccount.Nonce != 0 || len(newAccount.Code) > 0 {
			newBalance := oldAccount.Balance
			if newAccount.Balance != nil {
				newBalance = newAccount.Balance
			}
			newNonce := oldAccount.Nonce
			if newAccount.Nonce != 0 {
				newNonce = newAccount.Nonce
			}
			newCodeHash := crypto.Keccak256Hash(oldAccount.Code)
			if len(newAccount.Code) > 0 {
				newCodeHash = crypto.Keccak256Hash(newAccount.Code)
			}

			stateDiff.NewAccounts = append(stateDiff.NewAccounts, ptypes.NewAccount{
				Address:  addressToHash(addr),
				Balance:  uint256.MustFromBig(newBalance),
				Nonce:    newNonce,
				CodeHash: newCodeHash,
			})
		}
	}

	for addr := range t.deleted {
		stateDiff.DeletedAccounts = append(stateDiff.DeletedAccounts, addressToHash(addr))
	}

	for addr, acct := range t.post {
		Values := make([]ptypes.IndexValuePair, 0, len(acct.Storage))
		for k, v := range acct.Storage {
			value := uint256.NewInt(0).SetBytes(v.Bytes())
			Values = append(Values, ptypes.IndexValuePair{
				Index: crypto.Keccak256Hash(k[:]),
				Value: value,
			})
		}
		if len(Values) > 0 {
			stateDiff.StorageDiff = append(stateDiff.StorageDiff, ptypes.AccountStorageDiff{
				Address: addressToHash(addr),
				Values:  Values,
			})
		}
	}

	for _, code := range t.post {
		if len(code.Code) > 0 {
			stateDiff.NewCodes = append(stateDiff.NewCodes, ptypes.NewCode{
				CodeHash: crypto.Keccak256Hash(code.Code),
				Code:     code.Code,
			})
		}
	}
	return stateDiff
}

func (t *prestateTracer) OnBalanceChange(addr common.Address, prev, new *big.Int, reason tracing.BalanceChangeReason) {
	if _, ok := t.pre[addr]; ok {
		return
	}

	acc := &account{
		Balance: prev,
		Nonce:   t.env.StateDB.GetNonce(addr),
		Code:    t.env.StateDB.GetCode(addr),
		Storage: make(map[common.Hash]common.Hash),
	}
	if !acc.exists() {
		acc.empty = true
	}
	t.pre[addr] = acc
}

const (
	memoryPadLimit = 1024 * 1024
)

// GetMemoryCopyPadded returns offset + size as a new slice.
// It zero-pads the slice if it extends beyond memory bounds.
func GetMemoryCopyPadded(m []byte, offset, size int64) ([]byte, error) {
	if offset < 0 || size < 0 {
		return nil, errors.New("offset or size must not be negative")
	}
	length := int64(len(m))
	if offset+size < length { // slice fully inside memory
		return memoryCopy(m, offset, size), nil
	}
	paddingNeeded := offset + size - length
	if paddingNeeded > memoryPadLimit {
		return nil, fmt.Errorf("reached limit for padding memory slice: %d", paddingNeeded)
	}
	cpy := make([]byte, size)
	if overlap := length - offset; overlap > 0 {
		copy(cpy, MemoryPtr(m, offset, overlap))
	}
	return cpy, nil
}

func memoryCopy(m []byte, offset, size int64) (cpy []byte) {
	if size == 0 {
		return nil
	}

	if len(m) > int(offset) {
		cpy = make([]byte, size)
		copy(cpy, m[offset:offset+size])

		return
	}

	return
}

// MemoryPtr returns a pointer to a slice of memory.
func MemoryPtr(m []byte, offset, size int64) []byte {
	if size == 0 {
		return nil
	}

	if len(m) > int(offset) {
		return m[offset : offset+size]
	}

	return nil
}
