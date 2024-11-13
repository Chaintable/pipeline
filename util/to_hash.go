package util

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

func ToHash(args []string) (string, error) {
	hasher := md5.New()

	for _, arg := range args {
		if arg == "" {
			return "", fmt.Errorf("nil argument provided")
		}
		hasher.Write([]byte(arg))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
