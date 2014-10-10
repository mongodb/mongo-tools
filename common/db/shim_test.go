package db

import (
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"runtime"
	"testing"
)

const (
	windowsShimPath = `testdata\mock_shim.bat`
)

func TestShimRead(t *testing.T) {
	shimPath := "testdata/mock_shim.sh"
	if runtime.GOOS == "windows" {
		shimPath = windowsShimPath
	}
	Convey("Test shim process in read mode", t, func() {
		var bsonTool StorageShim
		resetFunc := func() {
			//invokes a fake shim that just cat's a bson file
			bsonTool = StorageShim{
				DBPath:     "fake db path",
				Database:   "fake database",
				Collection: "fake collection",
				Query:      "{}",
				Skip:       0,
				Limit:      0,
				Mode:       Dump,
				ShimPath:   shimPath,
			}
		}
		Reset(resetFunc)
		resetFunc()

		Convey("with raw byte stream", func() {
			iter, _, err := bsonTool.Open()
			if err != nil {
				t.Fatal(err)
			}

			docCount := 0
			docBytes := make([]byte, MaxBSONSize)
			for {
				if hasDoc, docSize := iter.LoadNextInto(docBytes); hasDoc {
					val := map[string]interface{}{}
					bson.Unmarshal(docBytes[0:docSize], val)
					docCount++
				} else {
					break
				}
			}
			So(iter.Err(), ShouldBeNil)
			So(docCount, ShouldEqual, 100)
		})

		Convey("with decoded byte stream", func() {
			iter, _, err := bsonTool.Open()
			if err != nil {
				t.Fatal(err)
			}

			decStrm := NewDecodedBSONSource(iter)
			docVal := bson.M{}
			docCount := 0
			for decStrm.Next(docVal) {
				docCount++
			}
			So(iter.Err(), ShouldBeNil)
			So(docCount, ShouldEqual, 100)
		})
	})
}

func TestShimWrite(t *testing.T) {
	shimPath := "testdata/mock_shim_write.sh"
	if runtime.GOOS == "windows" {
		shimPath = windowsShimPath
	}
	Convey("Test shim process in write mode", t, func() {
		var bsonTool StorageShim
		resetFunc := func() {
			//invokes a fake shim that just cat's a bson file
			bsonTool = StorageShim{
				DBPath:     "fake db path",
				Database:   "fake database",
				Collection: "fake collection",
				Query:      "{}",
				Skip:       0,
				Limit:      0,
				Mode:       Insert,
				ShimPath:   shimPath,
			}
		}
		Reset(resetFunc)
		resetFunc()

		Convey("with raw byte stream", func() {
			_, writer, err := bsonTool.Open()
			if err != nil {
				t.Fatal(err)
			}

			encodedSink := &EncodedBSONSink{writer, &bsonTool}
			err = encodedSink.WriteDoc(bson.M{"hi": "there"})
			So(err, ShouldBeNil)
		})
	})
}

func SkipTestShimCommand(t *testing.T) {

	Convey("Test running shim command", t, func() {
		shim, err := NewShim("testdata", false, false)
		if err != nil {
			t.Fatal(err)
		}
		out := bson.M{}
		err = shim.Run(bson.M{"listDatabases": 1}, &out, "admin")
		So(err, ShouldBeNil)
	})

}
