// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongorestore/ns"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	// bump up the verbosity to make checking debug log output possible
	log.SetVerbosity(&options.Verbosity{
		VLevel: 4,
	})
}

func newMongoRestore() *MongoRestore {
	renamer, _ := ns.NewRenamer([]string{}, []string{})
	includer, _ := ns.NewMatcher([]string{"*"})
	excluder, _ := ns.NewMatcher([]string{})
	return &MongoRestore{
		manager:      intents.NewIntentManager(),
		InputOptions: &InputOptions{},
		ToolOptions:  &commonOpts.ToolOptions{},
		NSOptions:    &NSOptions{},
		renamer:      renamer,
		includer:     includer,
		excluder:     excluder,
	}
}

func TestCreateAllIntents(t *testing.T) {
	// This tests creates intents based on the test file tree:
	//   testdirs/badfile.txt
	//   testdirs/oplog.bson
	//   testdirs/db1
	//   testdirs/db1/baddir
	//   testdirs/db1/baddir/out.bson
	//   testdirs/db1/c1.bson
	//   testdirs/db1/c1.metadata.json
	//   testdirs/db1/c2.bson
	//   testdirs/db1/c3.bson
	//   testdirs/db1/c3.metadata.json
	//   testdirs/db1/c4.bson
	//   testdirs/db1/c4.metadata.json
	//   testdirs/db2
	//   testdirs/db2/c1.bin
	//   testdirs/db2/c2.txt

	var mr *MongoRestore
	var buff bytes.Buffer

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a test MongoRestore", t, func() {
		mr = newMongoRestore()
		log.SetWriter(&buff)

		Convey("running CreateAllIntents should succeed", func() {
			ddl, err := newActualPath("testdata/testdirs/")
			So(err, ShouldBeNil)
			So(mr.CreateAllIntents(ddl), ShouldBeNil)
			mr.manager.Finalize(intents.Legacy)

			Convey("and reading the intents should show alphabetical order", func() {
				i0 := mr.manager.Pop()
				So(i0.DB, ShouldEqual, "db1")
				So(i0.C, ShouldEqual, "c1")
				i1 := mr.manager.Pop()
				So(i1.DB, ShouldEqual, "db1")
				So(i1.C, ShouldEqual, "c2")
				i2 := mr.manager.Pop()
				So(i2.DB, ShouldEqual, "db1")
				So(i2.C, ShouldEqual, "c3")
				i3 := mr.manager.Pop()
				So(i3.DB, ShouldEqual, "db1")
				So(i3.C, ShouldEqual, "c4")
				i4 := mr.manager.Pop()
				So(i4.DB, ShouldEqual, "db2")
				So(i4.C, ShouldEqual, "c1")
				i5 := mr.manager.Pop()
				So(i5, ShouldBeNil)

				Convey("with all the proper metadata + bson merges", func() {
					So(i0.Location, ShouldNotEqual, "")
					So(i0.MetadataLocation, ShouldNotEqual, "")
					So(i1.Location, ShouldNotEqual, "")
					So(i1.MetadataLocation, ShouldEqual, "") // no metadata for this file
					So(i2.Location, ShouldNotEqual, "")
					So(i2.MetadataLocation, ShouldNotEqual, "")
					So(i3.Location, ShouldNotEqual, "")
					So(i3.MetadataLocation, ShouldNotEqual, "")
					So(i4.Location, ShouldNotEqual, "")
					So(i4.MetadataLocation, ShouldEqual, "") // no metadata for this file

					Convey("and skipped files all present in the logs", func() {
						logs := buff.String()
						So(strings.Contains(logs, "badfile.txt"), ShouldEqual, true)
						So(strings.Contains(logs, "baddir"), ShouldEqual, true)
						So(strings.Contains(logs, "c2.txt"), ShouldEqual, true)
					})
				})
			})
		})
	})
}

func TestCreateAllIntentsLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	// This tests creates intents based on the test file tree:
	//   testdata/longcollectionname
	//   testdata/longcollectionname/db1
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.bson
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.metadata.json

	var mr *MongoRestore
	var buff bytes.Buffer

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a test MongoRestore", t, func() {
		mr = newMongoRestore()
		log.SetWriter(&buff)

		Convey("running CreateAllIntents should succeed", func() {
			ddl, err := newActualPath("testdata/longcollectionname/")
			So(err, ShouldBeNil)
			So(mr.CreateAllIntents(ddl), ShouldBeNil)
			mr.manager.Finalize(intents.Legacy)

			Convey("and reading the intents should show a long collection name", func() {
				i0 := mr.manager.Pop()
				So(i0.DB, ShouldEqual, "db1")
				So(i0.C, ShouldEqual, longCollectionName)

				Convey("with all the proper metadata + bson merges", func() {
					So(i0.Location, ShouldNotEqual, "")
					So(i0.MetadataLocation, ShouldNotEqual, "")
				})
			})
		})
	})
}

func TestCreateIntentsForDB(t *testing.T) {
	// This tests creates intents based on the test file tree:
	//   db1
	//   db1/baddir
	//   db1/baddir/out.bson
	//   db1/c1.bson
	//   db1/c1.metadata.json
	//   db1/c2.bson
	//   db1/c3.bson
	//   db1/c3.metadata.json
	//   db1/c4.bson
	//   db1/c4.metadata.json

	var mr *MongoRestore
	var buff bytes.Buffer

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a test MongoRestore", t, func() {
		mr = newMongoRestore()
		log.SetWriter(&buff)

		Convey("running CreateIntentsForDB should succeed", func() {
			ddl, err := newActualPath("testdata/testdirs/db1")
			So(err, ShouldBeNil)
			err = mr.CreateIntentsForDB("myDB", ddl)
			So(err, ShouldBeNil)
			mr.manager.Finalize(intents.Legacy)

			Convey("and reading the intents should show alphabetical order", func() {
				i0 := mr.manager.Pop()
				So(i0.C, ShouldEqual, "c1")
				i1 := mr.manager.Pop()
				So(i1.C, ShouldEqual, "c2")
				i2 := mr.manager.Pop()
				So(i2.C, ShouldEqual, "c3")
				i3 := mr.manager.Pop()
				So(i3.C, ShouldEqual, "c4")
				i4 := mr.manager.Pop()
				So(i4, ShouldBeNil)

				Convey("and all intents should have the supplied db name", func() {
					So(i0.DB, ShouldEqual, "myDB")
					So(i1.DB, ShouldEqual, "myDB")
					So(i2.DB, ShouldEqual, "myDB")
					So(i3.DB, ShouldEqual, "myDB")
				})

				Convey("with all the proper metadata + bson merges", func() {
					So(i0.Location, ShouldNotEqual, "")
					So(i0.MetadataLocation, ShouldNotEqual, "")
					So(i1.Location, ShouldNotEqual, "")
					So(i1.MetadataLocation, ShouldEqual, "") // no metadata for this file
					So(i2.Location, ShouldNotEqual, "")
					So(i2.MetadataLocation, ShouldNotEqual, "")
					So(i3.Location, ShouldNotEqual, "")
					So(i3.MetadataLocation, ShouldNotEqual, "")

					Convey("and skipped files all present in the logs", func() {
						logs := buff.String()
						So(strings.Contains(logs, "baddir"), ShouldEqual, true)
					})
				})
			})
		})
	})
}

func TestCreateIntentsForDBLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	// This tests creates intents based on the test file tree:
	//   testdata/longcollectionname/db1
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.bson
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.metadata.json

	var mr *MongoRestore
	var buff bytes.Buffer

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a test MongoRestore", t, func() {
		mr = newMongoRestore()
		log.SetWriter(&buff)

		Convey("running CreateIntentsForDB should succeed", func() {
			ddl, err := newActualPath("testdata/longcollectionname/db1")
			So(err, ShouldBeNil)
			err = mr.CreateIntentsForDB("myDB", ddl)
			So(err, ShouldBeNil)
			mr.manager.Finalize(intents.Legacy)

			Convey("and reading the intents should show alphabetical order", func() {
				i0 := mr.manager.Pop()
				So(i0.C, ShouldEqual, longCollectionName)

				Convey("and all intents should have the supplied db name", func() {
					So(i0.DB, ShouldEqual, "myDB")
				})

				Convey("with all the proper metadata + bson merges", func() {
					So(i0.Location, ShouldNotEqual, "")
					So(i0.MetadataLocation, ShouldNotEqual, "")
				})
			})
		})
	})
}

func TestCreateIntentsRenamed(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("With a test MongoRestore", t, func() {
		mr := newMongoRestore()
		mr.renamer, _ = ns.NewRenamer([]string{"db1.*"}, []string{"db4.test.*"})

		Convey("running CreateAllIntents should succeed", func() {
			ddl, err := newActualPath("testdata/testdirs/")
			So(err, ShouldBeNil)
			So(mr.CreateAllIntents(ddl), ShouldBeNil)
			mr.manager.Finalize(intents.Legacy)

			Convey("and reading the intents should show new collection names", func() {
				i0 := mr.manager.Pop()
				So(i0.C, ShouldEqual, "test.c1")
				i1 := mr.manager.Pop()
				So(i1.C, ShouldEqual, "test.c2")
				i2 := mr.manager.Pop()
				So(i2.C, ShouldEqual, "test.c3")
				i3 := mr.manager.Pop()
				So(i3.C, ShouldEqual, "test.c4")
				i4 := mr.manager.Pop()
				So(i4.C, ShouldEqual, "c1")
				i5 := mr.manager.Pop()
				So(i5, ShouldBeNil)

				Convey("and intents should have the renamed db", func() {
					So(i0.DB, ShouldEqual, "db4")
					So(i1.DB, ShouldEqual, "db4")
					So(i2.DB, ShouldEqual, "db4")
					So(i3.DB, ShouldEqual, "db4")
					So(i4.DB, ShouldEqual, "db2")
				})
			})
		})
	})
}

func TestHandlingBSON(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	var mr *MongoRestore
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a test MongoRestore", t, func() {
		mr = newMongoRestore()

		Convey("with a target path to a non-truncated bson file instead of a directory", func() {
			err := mr.handleBSONInsteadOfDirectory("testdata/testdirs/db1/c2.bson")
			So(err, ShouldBeNil)

			Convey("the proper DB and Coll should be inferred", func() {
				So(mr.ToolOptions.Namespace.DB, ShouldEqual, "db1")
				So(mr.ToolOptions.Namespace.Collection, ShouldEqual, "c2")
			})
		})

		Convey("with a target path to a truncated bson file instead of a directory", func() {
			err := mr.handleBSONInsteadOfDirectory(
				"testdata/longcollectionname/db1/" + longBsonName,
			)
			So(err, ShouldBeNil)

			Convey("the proper DB and Coll should be inferred", func() {
				So(mr.ToolOptions.Namespace.DB, ShouldEqual, "db1")
				So(mr.ToolOptions.Namespace.Collection, ShouldEqual, longCollectionName)
			})
		})

		Convey("but pre-existing settings should not be overwritten", func() {
			mr.ToolOptions.Namespace.DB = "a"

			Convey("either collection settings", func() {
				mr.ToolOptions.Namespace.Collection = "b"
				err := mr.handleBSONInsteadOfDirectory("testdata/testdirs/db1/c1.bson")
				So(err, ShouldBeNil)
				So(mr.ToolOptions.Namespace.DB, ShouldEqual, "a")
				So(mr.ToolOptions.Namespace.Collection, ShouldEqual, "b")
			})

			Convey("or db settings", func() {
				err := mr.handleBSONInsteadOfDirectory("testdata/testdirs/db1/c1.bson")
				So(err, ShouldBeNil)
				So(mr.ToolOptions.Namespace.DB, ShouldEqual, "a")
				So(mr.ToolOptions.Namespace.Collection, ShouldEqual, "c1")
			})
		})
	})
}

func TestCreateIntentsForCollection(t *testing.T) {
	var mr *MongoRestore
	var buff bytes.Buffer

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a test MongoRestore", t, func() {
		buff = bytes.Buffer{}
		mr = &MongoRestore{
			manager:      intents.NewIntentManager(),
			ToolOptions:  &commonOpts.ToolOptions{},
			InputOptions: &InputOptions{},
		}
		log.SetWriter(&buff)

		Convey("running CreateIntentForCollection on a file without metadata", func() {
			ddl, err := newActualPath(util.ToUniversalPath("testdata/testdirs/db1/c2.bson"))
			So(err, ShouldBeNil)
			err = mr.CreateIntentForCollection("myDB", "myC", ddl)
			So(err, ShouldBeNil)
			mr.manager.Finalize(intents.Legacy)

			Convey("should create one intent with 'myDb' and 'myC' fields", func() {
				i0 := mr.manager.Pop()
				So(i0, ShouldNotBeNil)
				So(i0.DB, ShouldEqual, "myDB")
				So(i0.C, ShouldEqual, "myC")
				ddl, err := newActualPath(util.ToUniversalPath("testdata/testdirs/db1/c2.bson"))
				So(err, ShouldBeNil)
				So(i0.Location, ShouldEqual, ddl.Path())
				i1 := mr.manager.Pop()
				So(i1, ShouldBeNil)

				Convey("and no Metadata path", func() {
					So(i0.MetadataLocation, ShouldEqual, "")
					logs := buff.String()
					So(strings.Contains(logs, "without metadata"), ShouldEqual, true)
				})
			})
		})

		Convey("running CreateIntentForCollection on a file *with* metadata", func() {
			ddl, err := newActualPath(util.ToUniversalPath("testdata/testdirs/db1/c1.bson"))
			So(err, ShouldBeNil)
			err = mr.CreateIntentForCollection("myDB", "myC", ddl)
			So(err, ShouldBeNil)
			mr.manager.Finalize(intents.Legacy)

			Convey("should create one intent with 'myDb' and 'myC' fields", func() {
				i0 := mr.manager.Pop()
				So(i0, ShouldNotBeNil)
				So(i0.DB, ShouldEqual, "myDB")
				So(i0.C, ShouldEqual, "myC")
				So(i0.Location, ShouldEqual, util.ToUniversalPath("testdata/testdirs/db1/c1.bson"))
				i1 := mr.manager.Pop()
				So(i1, ShouldBeNil)

				Convey("and a set Metadata path", func() {
					So(
						i0.MetadataLocation,
						ShouldEqual,
						util.ToUniversalPath("testdata/testdirs/db1/c1.metadata.json"),
					)
					logs := buff.String()
					So(strings.Contains(logs, "found metadata"), ShouldEqual, true)
				})
			})
		})

		Convey("running CreateIntentForCollection on a non-existent file", func() {
			_, err := newActualPath("aaaaaaaaaaaaaa.bson")
			Convey("should fail", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("running CreateIntentForCollection on a directory", func() {
			ddl, err := newActualPath("testdata")
			So(err, ShouldBeNil)
			err = mr.CreateIntentForCollection(
				"myDB", "myC", ddl)

			Convey("should fail", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("running CreateIntentForCollection on non-bson file", func() {
			ddl, err := newActualPath("testdata/testdirs/db1/c1.metadata.json")
			So(err, ShouldBeNil)
			err = mr.CreateIntentForCollection(
				"myDB", "myC", ddl)

			Convey("should fail", func() {
				So(err, ShouldNotBeNil)
			})
		})

	})
}

func TestCreateIntentsForLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	var mr *MongoRestore
	var buff bytes.Buffer

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a test MongoRestore", t, func() {
		buff = bytes.Buffer{}
		mr = &MongoRestore{
			manager:      intents.NewIntentManager(),
			ToolOptions:  &commonOpts.ToolOptions{},
			InputOptions: &InputOptions{},
		}
		log.SetWriter(&buff)

		Convey(
			"running CreateIntentForCollection on a truncated bson file without metadata",
			func() {
				ddl, err := newActualPath(
					util.ToUniversalPath("testdata/longcollectionname/" + longInvalidBson),
				)
				So(err, ShouldBeNil)
				err = mr.CreateIntentForCollection("myDB", "myC", ddl)

				Convey("should fail", func() {
					So(err, ShouldNotBeNil)
				})
			},
		)

		Convey(
			"running CreateIntentForCollection on a truncated bson file *with* metadata",
			func() {
				ddl, err := newActualPath(
					util.ToUniversalPath("testdata/longcollectionname/db1/" + longBsonName),
				)
				So(err, ShouldBeNil)
				err = mr.CreateIntentForCollection("myDB", "myC", ddl)
				So(err, ShouldBeNil)
				mr.manager.Finalize(intents.Legacy)

				Convey("should create one intent with 'myDb' and 'myC' fields", func() {
					i0 := mr.manager.Pop()
					So(i0, ShouldNotBeNil)
					So(i0.DB, ShouldEqual, "myDB")
					So(i0.C, ShouldEqual, "myC")
					So(
						i0.Location,
						ShouldEqual,
						util.ToUniversalPath("testdata/longcollectionname/db1/"+longBsonName),
					)
					i1 := mr.manager.Pop()
					So(i1, ShouldBeNil)

					Convey("and a set Metadata path", func() {
						So(
							i0.MetadataLocation,
							ShouldEqual,
							util.ToUniversalPath(
								"testdata/longcollectionname/db1/"+longMetadataName,
							),
						)
						logs := buff.String()
						So(strings.Contains(logs, "found metadata"), ShouldEqual, true)
					})
				})
			},
		)
	})
}
