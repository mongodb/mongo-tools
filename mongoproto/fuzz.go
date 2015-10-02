// +build gofuzz

package mongoproto

import (
	"bytes"
)

func Fuzz(data []byte) int {
	if _, err := OpFromReader(bytes.NewReader(data)); err != nil {
		return 0
	}
	return 1
}
