package bsonutil

import (
	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"testing"
)

func TestBson2Float64(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	decimalVal, _ := primitive.ParseDecimal128("-1")
	tests := []struct {
		In        interface{}
		Expected  float64
		isSuccess bool
	}{
		{int32(1), 1.0, true},
		{int64(1), 1.0, true},
		{1.0, 1.0, true},
		{decimalVal, float64(-1), true},
		{"invalid value", 0, false},
	}

	Convey("Test numerical value conversion", t, func() {
		for _, test := range tests {
			result, ok := Bson2Float64(test.In)
			So(ok, ShouldEqual, test.isSuccess)
			So(result, ShouldEqual, test.Expected)
		}
	})
}
