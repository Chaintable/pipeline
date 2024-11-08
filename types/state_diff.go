package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

type NewAccount struct {
	Address  common.Hash
	Balance  *uint256.Int
	Nonce    uint64
	CodeHash common.Hash
}

type NewCode struct {
	CodeHash common.Hash
	Code     []byte
}

type IndexValuePair struct {
	Index common.Hash
	Value *uint256.Int
}

type AccountStorageDiff struct {
	Address common.Hash
	Values  []IndexValuePair
}

type BlockStorageDiff struct {
	Hash            common.Hash
	ParentHash      common.Hash
	NewAccounts     []NewAccount
	DeletedAccounts []common.Hash
	StorageDiff     []AccountStorageDiff
	NewCodes        []NewCode
}
