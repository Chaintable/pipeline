package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Preimages struct {
	Preimages []hexutil.Bytes `json:"preimages"`
	TxID      string          `json:"tx_id"`
}

type SlotChanges struct {
	Changes map[common.Address]map[common.Hash]SlotChange `json:"changes"`
	TxID    string                                        `json:"tx_id"`
}

type SlotChange struct {
	Pre  common.Hash `json:"pre"`
	Post common.Hash `json:"post"`
}
