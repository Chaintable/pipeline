package processor

import (
	"github.com/DeBankDeFi/pipeline/store"
	"github.com/DeBankDeFi/pipeline/types"
	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/lru"
)

const (
	ExtraInfoCacheSize = 1024
)

type ExtraInfo struct {
	BlockNumber     uint64
	BlockHash       common.Hash
	Traces          map[common.Hash][]types.CallFrame
	EventIndex      []types.EventPosition
	BlockDiff       *types.BlockStorageDiff
	TotalEventCount uint64
}

type ExtraInfoProcessor struct {
	RecentExtraInfos *lru.Cache[common.Hash, *ExtraInfo]
	store            *pebble.DB
}

func NewExtraInfoProcessor(path string) (*ExtraInfoProcessor, error) {
	store, err := store.Init(path, 1024*1024*1024)
	if err != nil {
		return nil, err
	}
	return &ExtraInfoProcessor{
		RecentExtraInfos: lru.NewCache[common.Hash, *ExtraInfo](ExtraInfoCacheSize),
		store:            store,
	}, nil
}

func (p *ExtraInfoProcessor) AddExtraInfo(blockNumber uint64, blockHash common.Hash, extraInfo *ExtraInfo) {
	p.RecentExtraInfos.Add(blockHash, extraInfo)
	batch := p.store.NewBatch()
	defer batch.Close()
	store.WriteBlockEventCount(batch, blockNumber, blockHash, extraInfo.TotalEventCount)
	store.WriteDiffRlp(batch, blockNumber, blockHash, extraInfo.BlockDiff)
	for txHash, traces := range extraInfo.Traces {
		store.WriteTrace(batch, blockNumber, blockHash, txHash, traces)
	}
	store.WriteEventIndex(batch, blockNumber, blockHash, extraInfo.EventIndex)
	batch.Commit(nil)
}

func (p *ExtraInfoProcessor) GetExtraInfo(blockNumber uint64, blockHash common.Hash) (*ExtraInfo, error) {
	extraInfo, ok := p.RecentExtraInfos.Get(blockHash)
	if ok {
		return extraInfo, nil
	}
	blockEventCount, err := store.ReadBlockEventCount(p.store, blockNumber, blockHash)
	if err != nil {
		return nil, err
	}
	blockDiff, err := store.ReadDiffRlp(p.store, blockNumber, blockHash)
	if err != nil {
		return nil, err
	}
	eventIndex, err := store.ReadEventIndex(p.store, blockNumber, blockHash)
	if err != nil {
		return nil, err
	}
	traces, err := store.ReadBlockAllTraces(p.store, blockNumber, blockHash)
	if err != nil {
		return nil, err
	}
	return &ExtraInfo{
		BlockNumber:     blockNumber,
		BlockHash:       blockHash,
		Traces:          traces,
		EventIndex:      eventIndex,
		BlockDiff:       blockDiff,
		TotalEventCount: blockEventCount,
	}, nil
}

func (p *ExtraInfoProcessor) GetBlockEventCount(blockNumber uint64, blockHash common.Hash) (uint64, error) {
	if extraInfo, ok := p.RecentExtraInfos.Get(blockHash); ok {
		return extraInfo.TotalEventCount, nil
	}
	return store.ReadBlockEventCount(p.store, blockNumber, blockHash)
}

func (p *ExtraInfoProcessor) Close() error {
	return p.store.Close()
}
