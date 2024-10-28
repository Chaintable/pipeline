package store

import (
	"github.com/cockroachdb/pebble"
)

func Init(path string) (*pebble.DB, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, err
	}
	return db, nil
}
