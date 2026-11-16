package processor

import (
	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
)

// BlockChainReader defines minimal interface for reading blockchain headers
// This allows pipeline to compute common ancestor without depending on full blockchain implementation
type BlockChainReader interface {
	// GetHeaderByHash2 retrieves a block header from the database by hash
	// If not found locally, it may fetch from S3 (implementation-specific)
	GetHeaderByHash2(hash common.Hash) *types.Header
}
