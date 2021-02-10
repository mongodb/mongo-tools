package password

import (
	"bytes"
	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

const (
	testPwd = "test_pwd"
)

func TestPasswordFromNonTerminal(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("stdin is not a terminal", t, func() {
		var buffer bytes.Buffer

		buffer.WriteString(testPwd)
		reader := bytes.NewReader(buffer.Bytes())

		pass, err := readPassNonInteractively(reader)
		So(err, ShouldBeNil)
		So(pass, ShouldEqual, testPwd)
	})
}
