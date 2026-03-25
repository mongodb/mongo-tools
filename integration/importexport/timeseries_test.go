package importexport

import (
	"bytes"
	"os"

	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (s *ImportExportSuite) TestTimeseries() {
	s.RequireFCVAtLeast("5.0")

	client := s.Client()
	serverVersion := s.ServerVersion()

	fromDBName, toDBName, collName := "fromdb", "todb", "tscoll"

	cleanup := func() {
		err := client.Database(fromDBName).Drop(s.T().Context())
		s.Require().NoError(err)

		err = client.Database(toDBName).Drop(s.T().Context())
		s.Require().NoError(err)
	}

	cleanup()
	defer cleanup()

	testutil.SetUpTimeseries(s.T(), fromDBName, collName)

	s.Run("logical documents", func() {
		buf := new(bytes.Buffer)

		s.Run("export", func() {
			opts := s.ExportOptions()

			opts.Collection = collName
			opts.DB = fromDBName

			me, err := mongoexport.New(opts)
			s.Require().NoError(err)
			defer me.Close()
			count, err := me.Export(buf)
			s.Require().NoError(err)
			s.Assert().EqualValues(1000, count)
		})

		s.Run("import", func() {
			file := testutil.WriteTempFile(s.T(), buf)
			defer os.Remove(file.Name())

			createCmd := bson.D{
				{"create", collName},
				{"timeseries", bson.D{
					{"timeField", "ts"},
					{"metaField", "my_meta"},
				}},
			}

			db := client.Database(toDBName)
			res := db.RunCommand(s.T().Context(), createCmd)
			s.Require().NoError(res.Err(), "create timeseries coll")

			opts := s.ImportOptions(toDBName, collName)
			opts.InputOptions.File = file.Name()

			imp, err := mongoimport.New(opts)
			s.Require().NoError(err)

			numProcessed, _, err := imp.ImportDocuments()
			s.Require().NoError(err)
			s.Assert().EqualValues(1000, numProcessed)
		})
	})

	s.Run("bucket documents", func() {
		buf := new(bytes.Buffer)

		s.Run("export", func() {
			opts := s.ExportOptions()

			opts.Collection = "system.buckets." + collName
			opts.DB = fromDBName

			me, err := mongoexport.New(opts)
			s.Require().NoError(err)
			defer me.Close()

			count, err := me.Export(buf)
			if serverVersion.SupportsRawData() {
				s.Assert().Zero(count)
				s.Require().ErrorContains(
					err,
					"does not support exporting system.buckets collections",
				)
			} else {
				s.Require().NoError(err)
				s.Assert().EqualValues(10, count)
			}

		})

		s.Run("import", func() {
			file := testutil.WriteTempFile(s.T(), buf)
			defer os.Remove(file.Name())

			opts := s.ImportOptions(toDBName, "system.buckets."+collName)
			opts.InputOptions.File = file.Name()

			_, err := mongoimport.New(opts)
			s.Require().Error(err)
			s.Assert().ErrorContains(err, "not allowed to begin with 'system.'")
		})
	})
}
