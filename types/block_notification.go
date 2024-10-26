package types

import (
	"github.com/ethereum/go-ethereum/common"
)

type BlockChangeNotification struct {
	ChangeType uint64         `json:"changeType"` // 1 for new, 2 for fork
	NewBlocks  []BlockContext `json:"newBlocks"`  // new block, 按高度排序
	DropBlocks []BlockContext `json:"dropBlocks"` // 由于fork，需要drop的block，按高度排序
}

type BlockContext struct {
	Hash        common.Hash `json:"hash"`
	ParrentHash common.Hash `json:"parrentHash"`
	BlockNumber uint64      `json:"blockNumber"`
}
