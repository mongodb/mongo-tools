//go:build unit

package password

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testPwd = "test_pwd"
)

func TestPasswordFromNonTerminal(t *testing.T) {
	var buffer bytes.Buffer

	buffer.WriteString(testPwd)
	reader := bytes.NewReader(buffer.Bytes())

	pass, err := readPassNonInteractively(reader)
	require.NoError(t, err)
	require.Equal(t, testPwd, pass)
}
