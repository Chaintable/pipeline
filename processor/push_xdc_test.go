package processor

import (
	"math/big"
	"strings"
	"testing"

	"github.com/Chaintable/pipeline/failover"
	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/XinFinOrg/XDPoSChain/common"
	"github.com/XinFinOrg/XDPoSChain/core/types"
)

func TestNotifyBlockCommitAcceptsXDCBlock(t *testing.T) {
	previous := failover.GlobalManager
	failover.GlobalManager = nil
	t.Cleanup(func() { failover.GlobalManager = previous })
	block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(0), Time: big.NewInt(123)})
	var p PushProcessor
	// This also checks that the API accepts XDC's *big.Int block timestamp.
	if err := p.NotifyBlockCommit(block, nil, nil); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("uninitialized processor must fail before any I/O: %v", err)
	}
}

type xdcHeaderReader map[common.Hash]*types.Header

func (r xdcHeaderReader) GetHeaderByHash2(hash common.Hash) *types.Header { return r[hash] }

func TestCommonAncestorPreservesXDCTimestamps(t *testing.T) {
	genesis := &types.Header{Number: big.NewInt(0), Time: big.NewInt(10)}
	parent := &types.Header{Number: big.NewInt(1), Time: big.NewInt(20), ParentHash: genesis.Hash()}
	reader := xdcHeaderReader{genesis.Hash(): genesis, parent.Hash(): parent}
	var p PushProcessor
	ancestor, dropped, added, err := p.getCommonAncestor(reader,
		ptypes.BlockContext{Hash: genesis.Hash(), BlockNumber: 0, Timestamp: 10},
		ptypes.BlockContext{Hash: common.Hash{3}, ParentHash: parent.Hash(), BlockNumber: 2, Timestamp: 30},
	)
	if err != nil || ancestor.Hash != genesis.Hash() || len(dropped) != 0 || len(added) != 2 || added[0].Timestamp != 20 || added[1].Timestamp != 30 {
		t.Fatalf("incorrect XDC ancestor conversion: ancestor=%+v dropped=%v added=%v err=%v", ancestor, dropped, added, err)
	}
}
