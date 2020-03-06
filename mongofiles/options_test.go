// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

import (
	"testing"

	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

func TestWriteConcernOptionParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Testing write concern parsing from command line and URI", t, func() {
		Convey("Parsing with neither URI nor command line option should set a majority write concern", func() {
			opts, err := ParseOptions([]string{}, "", "")

			So(err, ShouldBeNil)
			So(opts.StorageOptions.WriteConcern, ShouldEqual, "")
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		Convey("Parsing with URI with no write concern specified in it should set a majority write concern", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test",
			}
			opts, err := ParseOptions(args, "", "")

			So(err, ShouldBeNil)
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		Convey("Parsing with writeconcern only in URI should set it correctly", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test?w=2",
			}
			opts, err := ParseOptions(args, "", "")

			So(err, ShouldBeNil)
			So(opts.StorageOptions.WriteConcern, ShouldEqual, "")
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.W(2)))
		})

		Convey("Parsing with writeconcern only in command line should set it correctly", func() {
			args := []string{
				"--writeConcern", "{w: 2, j: true}",
			}
			opts, err := ParseOptions(args, "", "")

			So(err, ShouldBeNil)
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.W(2), writeconcern.J(true)))
		})
	})
}

type PositionalArgumentTestCase struct {
	InputArgs    []string
	ExpectedOpts Options
	ExpectedMF   MongoFiles
	ExpectErr    string
}

func TestPositionalArgumentParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Testing parsing positional arguments", t, func() {
		positionalArgumentTestCases := []PositionalArgumentTestCase{
			{
				InputArgs: []string{"list", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "list",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "list", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "list",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "list"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "",
					Command:  "list",
				},
			},
			{
				InputArgs: []string{"search", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "search",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "search", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "search",
				},
			},
			{
				InputArgs: []string{"put", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "put",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "put", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "put",
				},
			},
			{
				InputArgs: []string{"put_id", "foo", "id"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "put_id",
					Id:       "id",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "put_id", "foo", "id"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "put_id",
					Id:       "id",
				},
			},
			{
				InputArgs: []string{"get", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "get",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "get", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "get",
				},
			},
			{
				InputArgs: []string{"get_id", "id"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					Command: "get_id",
					Id:      "id",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "get_id", "id"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					Command: "get_id",
					Id:      "id",
				},
			},
			{
				InputArgs: []string{"delete", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "delete",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "delete", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					FileName: "foo",
					Command:  "delete",
				},
			},
			{
				InputArgs: []string{"delete_id", "id"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
				},
				ExpectedMF: MongoFiles{
					Command: "delete_id",
					Id:      "id",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "delete_id", "id"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
				ExpectedMF: MongoFiles{
					Command: "delete_id",
					Id:      "id",
				},
			},
			{
				InputArgs: []string{"put_id", "mongodb://foo", "mongodb://bar"},
				ExpectErr: "too many URIs found in positional arguments: only one URI can be set as a positional argument",
			},
			{
				InputArgs: []string{"list", "foo", "bar"},
				ExpectErr: "too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
			},
			{
				InputArgs: []string{"list", "foo", "bar", "mongodb://foo"},
				ExpectErr: "too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
			},
			{
				InputArgs: []string{"get"},
				ExpectErr: "'get' argument missing",
			},
			{
				InputArgs: []string{"get", "mongodb://foo"},
				ExpectErr: "'get' argument missing",
			},
			{
				InputArgs: []string{"foo", "bar"},
				ExpectErr: "'foo' is not a valid command (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
			},
			{
				InputArgs: []string{"list", "mongodb://foo", "--uri=mongodb://bar"},
				ExpectErr: "illegal argument combination: cannot specify a URI in a positional argument and --uri",
			},
		}

		for _, tc := range positionalArgumentTestCases {
			t.Logf("Testing: %s", tc.InputArgs)
			var mf *MongoFiles
			opts, err := ParseOptions(tc.InputArgs, "", "")

			if err == nil {
				mf = &MongoFiles{
					ToolOptions: opts.ToolOptions,
					StorageOptions: &StorageOptions{
						GridFSPrefix: "fs",
					},
				}
				err = mf.ValidateCommand(opts.ParsedArgs)
			}

			if tc.ExpectErr != "" {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, tc.ExpectErr)
			} else {
				So(err, ShouldBeNil)
				So(mf.FileName, ShouldEqual, tc.ExpectedMF.FileName)
				So(mf.Command, ShouldEqual, tc.ExpectedMF.Command)
				So(mf.Id, ShouldEqual, tc.ExpectedMF.Id)
				So(opts.ConnectionString, ShouldEqual, tc.ExpectedOpts.ConnectionString)
			}

		}
	})
}
