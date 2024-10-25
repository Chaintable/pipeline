package store

import (
	"runtime"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
)

func Init(path string, dbCacheSize int64) (*pebble.DB, error) {
	if dbCacheSize == 0 {
		dbCacheSize = 256
	}
	memTableLimit := 2
	memTableSize := int(dbCacheSize) * 1024 * 1024 / 2 / memTableLimit
	opt := &pebble.Options{
		// Pebble has a single combined cache area and the write
		// buffers are taken from this too. Assign all available
		// memory allowance for cache.
		Cache: pebble.NewCache(int64(dbCacheSize * 1024 * 1024)),

		// The size of memory table(as well as the write buffer).
		// Note, there may have more than two memory tables in the system.
		MemTableSize: uint64(memTableSize),

		MemTableStopWritesThreshold: memTableLimit,

		// The default compaction concurrency(1 thread),
		// Here use all available CPUs for faster compaction.
		MaxConcurrentCompactions: func() int { return runtime.NumCPU() },
		Levels: []pebble.LevelOptions{
			{TargetFileSize: 2 * 5 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 10 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		},
	}
	opt.Experimental.ReadSamplingMultiplier = -1
	db, err := pebble.Open(path, opt)
	if err != nil {
		return nil, err
	}
	return db, nil
}
