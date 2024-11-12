package util

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

func ToHash(args ...interface{}) (string, error) {
	hasher := md5.New()

	for _, arg := range args {
		if arg == nil {
			return "", fmt.Errorf("nil argument provided")
		}
		var str string
		switch v := arg.(type) {
		case string:
			str = v
		default:
			str = fmt.Sprintf("%v", v)
		}

		hasher.Write([]byte(str))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
