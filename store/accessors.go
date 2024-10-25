package store

import (
	"github.com/DeBankDeFi/pipeline/types"
	"github.com/DeBankDeFi/pipeline/util"
	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

func WriteBlockEventCount(batch *pebble.Batch, blockNumber uint64, blockHash common.Hash, count uint64) error {
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, BlockEventCountPrefix...)
	return batch.Set(key, EncodeNumber(count), nil)
}

func ReadBlockEventCount(store *pebble.DB, blockNumber uint64, blockHash common.Hash) (uint64, error) {
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, BlockEventCountPrefix...)
	value, closer, err := store.Get(key)
	defer closer.Close()
	if err != nil {
		return 0, err
	}
	return DecodeNumber(value), nil
}

func WriteDiffRlp(batch *pebble.Batch, blockNumber uint64, blockHash common.Hash, diff *types.BlockStorageDiff) error {
	buf, err := util.EncodeToRlp(diff)
	if err != nil {
		return err
	}
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, DiffPrefix...)
	return batch.Set(key, buf, nil)
}

func ReadDiffRlp(store *pebble.DB, blockNumber uint64, blockHash common.Hash) (*types.BlockStorageDiff, error) {
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, DiffPrefix...)
	value, closer, err := store.Get(key)
	defer closer.Close()
	if err != nil {
		return nil, err
	}
	diff := &types.BlockStorageDiff{}
	err = util.DecodeFromRlp(value, diff)
	if err != nil {
		return nil, err
	}
	return diff, nil
}

func WriteTrace(batch *pebble.Batch, blockNumber uint64, blockHash common.Hash, txHash common.Hash, traces []types.CallFrame) error {
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, TracePrefix...)
	key = append(key, txHash.Bytes()...)
	buf, err := util.EncodeToJsonGzip(traces)
	if err != nil {
		return err
	}
	return batch.Set(key, buf, nil)
}

func ReadTrace(store *pebble.DB, blockNumber uint64, blockHash common.Hash, txHash common.Hash) ([]types.CallFrame, error) {
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, TracePrefix...)
	key = append(key, txHash.Bytes()...)
	value, closer, err := store.Get(key)
	defer closer.Close()
	if err != nil {
		return nil, err
	}
	traces := []types.CallFrame{}
	err = util.DecodeFromGzipJson(value, &traces)
	if err != nil {
		return nil, err
	}
	return traces, nil
}

func ReadBlockAllTraces(store *pebble.DB, blockNumber uint64, blockHash common.Hash) (map[common.Hash][]types.CallFrame, error) {
	prefix := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	prefix = append(prefix, TracePrefix...)
	iter, err := store.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefix,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	traces := map[common.Hash][]types.CallFrame{}
	for iter.First(); iter.Valid(); iter.Next() {
		trace := []types.CallFrame{}
		err = util.DecodeFromGzipJson(iter.Value(), &trace)
		if err != nil {
			return nil, err
		}
		txHash := common.BytesToHash(iter.Key()[len(prefix):])
		traces[txHash] = trace
	}
	return traces, nil
}

func WriteEventIndex(batch *pebble.Batch, blockNumber uint64, blockHash common.Hash, eventPosition []types.EventPosition) error {
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, EventIndexPrefix...)
	if len(eventPosition) == 0 {
		log.Error("No event position to write")
		return nil
	}
	buf, err := util.EncodeToRlp(eventPosition)
	if err != nil {
		return err
	}
	return batch.Set(key, buf, nil)
}

func ReadEventIndex(store *pebble.DB, blockNumber uint64, blockHash common.Hash) ([]types.EventPosition, error) {
	key := append(EncodeNumber(blockNumber), blockHash.Bytes()...)
	key = append(key, EventIndexPrefix...)
	value, closer, err := store.Get(key)
	defer closer.Close()
	if err != nil {
		return nil, err
	}
	eventPosition := []types.EventPosition{}
	err = util.DecodeFromRlp(value, &eventPosition)
	if err != nil {
		return nil, err
	}
	return eventPosition, nil
}
