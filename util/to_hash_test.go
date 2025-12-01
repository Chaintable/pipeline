package util

import (
	"fmt"
	"testing"
)

func TestToHash(t *testing.T) {
	event_id := ToHash([]string{"abcd", "2"})
	if event_id != "6e24a85785fd5e2688f1a23aee9d88f3" {
		t.Errorf("Expected 6e24a85785fd5e2688f1a23aee9d88f3, got %s", event_id)
	}
}

func TestToHash2(t *testing.T) {
	event_id := ToHash([]string{"0xe9e91f1ee4b56c0df2e9f06c2b8c27c6076195a88a7b8537ba8313d80e6f124e", "", "0"})
	fmt.Println(event_id)
}
