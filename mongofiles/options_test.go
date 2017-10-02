package mongofiles

import (
	"fmt"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	. "github.com/smartystreets/goconvey/convey"
)

// Regression test for TOOLS-1741
func TestWriteConcernWithURIParsing(t *testing.T) {
	Convey("With an IngestOptions and ToolsOptions", t, func() {

		// create an 'EnabledOptions' to determine what options should be able to be
		// parsed and set form the input.
		enabled := options.EnabledOptions{URI: true}

		// create a new tools options to hold the parsed options
		opts := options.New("", "", enabled)

		// create a 'StorageOptions', which holds the value of the write concern
		// for mongofiles.
		storageOpts := &StorageOptions{}
		opts.AddOptions(storageOpts)

		// Specify that a write concern set on the URI is not an error and is a known
		// possible option.
		opts.URI.AddKnownURIParameters(options.KnownURIOptionsWriteConcern)

		Convey("Parsing with no value should leave write concern empty", func() {
			_, err := opts.ParseArgs([]string{})
			So(err, ShouldBeNil)
			So(storageOpts.WriteConcern, ShouldEqual, "")
			Convey("and building write concern object, WMode should be majority", func() {
				sessionSafety, err := db.BuildWriteConcern(storageOpts.WriteConcern, "",
					opts.ParsedConnString())
				So(err, ShouldBeNil)
				So(sessionSafety.WMode, ShouldEqual, "majority")
			})
		})

		Convey("Parsing with no writeconcern in URI should not error", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test",
			}
			_, err := opts.ParseArgs(args)
			So(err, ShouldBeNil)
			So(storageOpts.WriteConcern, ShouldEqual, "")
			Convey("and parsing write concern, WMode should be majority", func() {
				sessionSafety, err := db.BuildWriteConcern(storageOpts.WriteConcern, "",
					opts.ParsedConnString())
				So(err, ShouldBeNil)
				So(sessionSafety, ShouldNotBeNil)
				So(sessionSafety.WMode, ShouldEqual, "majority")
			})
		})
		Convey("Parsing with both writeconcern in URI and command line should error", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test",
				"--writeConcern", "majority",
			}
			_, err := opts.ParseArgs(args)
			So(err, ShouldBeNil)
			So(storageOpts.WriteConcern, ShouldEqual, "majority")
			Convey("and parsing write concern, WMode should be majority", func() {
				_, err := db.BuildWriteConcern(storageOpts.WriteConcern, "",
					opts.ParsedConnString())
				So(err, ShouldResemble, fmt.Errorf("cannot specify writeConcern string and connectionString object"))
			})
		})
	})
}
