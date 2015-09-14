package mongoproto_test

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/gabrielrussell/mongocaputils/mongoproto"
)

var testCorpusPath = "workdir/corpus/mongodb*"

func TestWiresharkPcapExamples(t *testing.T) {
	paths, err := filepath.Glob(testCorpusPath)
	if err != nil {
		t.Log("error opening corpus:", err)
		t.Skip()
	}
	for _, path := range paths {
		r, err := ioutil.ReadFile(path)
		if err != nil {
			t.Error(err)
			continue
		}
		_, err = mongoproto.OpFromReader(bytes.NewReader(r))
		if err != nil {
			t.Log(path)
			t.Fatal(err)
		}
	}
}
