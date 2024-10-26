package store

import (
	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
)

func WriteBlockEventCount(batch *pebble.Batch, blockHash common.Hash, count uint64) error {
	key := append(blockHash.Bytes(), BlockEventCountPrefix...)
	return batch.Set(key, EncodeNumber(count), nil)
}

func ReadBlockEventCount(store *pebble.DB, blockHash common.Hash) (uint64, error) {
	key := append(blockHash.Bytes(), BlockEventCountPrefix...)
	value, closer, err := store.Get(key)
	defer closer.Close()
	if err == pebble.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return DecodeNumber(value), nil
}
