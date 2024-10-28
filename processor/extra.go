package processor

import (
	"github.com/DeBankDeFi/pipeline/store"
	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
)

type ExtraInfoProcessor struct {
	Store *pebble.DB
}

func NewExtraInfoProcessor(path string) (*ExtraInfoProcessor, error) {
	store, err := store.Init(path)
	if err != nil {
		return nil, err
	}
	return &ExtraInfoProcessor{
		Store: store,
	}, nil
}

func (p *ExtraInfoProcessor) WriteBlockEventCount(blockHash common.Hash, totalEventCount uint64) error {
	batch := p.Store.NewBatch()
	defer batch.Close()
	store.WriteBlockEventCount(batch, blockHash, totalEventCount)
	return batch.Commit(nil)
}

func (p *ExtraInfoProcessor) GetBlockEventCount(blockHash common.Hash) (uint64, error) {
	return store.ReadBlockEventCount(p.Store, blockHash)
}

func (p *ExtraInfoProcessor) Close() error {
	return p.Store.Close()
}
