package exportimport

import (
	"bytes"
	"os"

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (s *ExportImportSuite) TestTimeseries() {
	s.RequireFCVAtLeast("5.0")

	s.WithCrossCluster(func(cc crossCluster) {
		sourceProvider, _, err := testutil.GetBareSessionProviderForURI(s.T(), cc.SourceURI)
		s.Require().NoError(err)
		serverVersion, err := sourceProvider.ServerVersionArray()
		s.Require().NoError(err)

		fromDBName, toDBName, collName := "fromdb", "todb", "tscoll"
		testutil.SetUpTimeseriesForURI(s.T(), cc.SourceURI, fromDBName, collName)

		s.Run("logical documents", func() {
			buf := new(bytes.Buffer)

			s.Run("export", func() {
				opts := s.ExportOptionsForURI(cc.SourceURI)

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

				db := cc.Target.Database(toDBName)
				res := db.RunCommand(s.Context(), createCmd)
				s.Require().NoError(res.Err(), "create timeseries coll")

				opts := s.ImportOptionsForURI(cc.TargetURI, toDBName, collName)
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
				opts := s.ExportOptionsForURI(cc.SourceURI)

				opts.Collection = common.TimeseriesBucketPrefix + collName
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

				opts := s.ImportOptionsForURI(
					cc.TargetURI,
					toDBName,
					common.TimeseriesBucketPrefix+collName,
				)
				opts.InputOptions.File = file.Name()

				_, err := mongoimport.New(opts)
				s.Require().Error(err)
				s.Assert().ErrorContains(err, "not allowed to begin with 'system.'")
			})
		})
	})
}
