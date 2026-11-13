package processor

import (
	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/types"
)

// BlockChainReader defines minimal interface for reading blockchain headers
// This allows pipeline to compute common ancestor without depending on full blockchain implementation
type BlockChainReader interface {
	// GetHeaderByHash2 retrieves a block header from the database by hash
	// If not found locally, it may fetch from S3 (implementation-specific)
	GetHeaderByHash2(hash common.Hash) *types.Header
}

// BlockContextReader can optionally be implemented by a BlockChainReader whose
// exported headers cannot be re-hashed from their fields (e.g. avax SAE headers
// carry the post-execution state root). getCommonAncestor then uses it instead
// of GetHeaderByHash2 so the stored block hash is kept.
type BlockContextReader interface {
	GetBlockContextByHash(hash common.Hash) (ptypes.BlockContext, error)
}
