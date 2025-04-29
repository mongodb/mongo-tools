package util

import (
	"bytes"
	"io"
)

func WriteAll(writer io.Writer, buffer []byte) error {
	_, err := io.Copy(writer, bytes.NewReader(buffer))

	return err
}
