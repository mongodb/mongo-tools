// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build integration || sharded

package mongoimport

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/stretchr/testify/suite"
)

type writeConcernSuite struct {
	sharedsuite.IntegrationSuite
}

// TestImportWriteConcern checks that mongoimport succeeds or fails according to
// whether the replica set can satisfy the write concern it was given, and that
// without a timeout it waits for the secondaries rather than giving up.
func TestImportWriteConcern(t *testing.T) {
	suite.Run(t, new(writeConcernSuite))
}

// TestSatisfiability runs the write-concern matrix against an import: the cases
// the replica set can satisfy succeed, and the ones it cannot fail with a write
// concern error.
func (s *writeConcernSuite) TestSatisfiability() {
	testutil.SkipUnlessReplicaSet(s.T())

	const collName = "bar"

	dbName := s.DBName()
	importFile := s.writeImportFile()

	s.RunWriteConcernCases(dbName, func(writeConcern string) error {
		return s.importWithWriteConcern(dbName, collName, importFile, writeConcern)
	})
}

// TestWaitsForSlowReplication checks that a write concern with no timeout makes
// mongoimport wait for the secondaries rather than giving up. It tests waiting
// rather than failing, so it has to run the tool concurrently with the
// failpoint.
func (s *writeConcernSuite) TestWaitsForSlowReplication() {
	testutil.SkipUnlessReplicaSet(s.T())

	const collName = "bar"

	dbName := s.DBName()
	importFile := s.writeImportFile()

	s.AssertWaitsForReplication("mongoimport", func(writeConcern string) func() error {
		mi := s.newImport(dbName, collName, importFile, writeConcern)
		s.T().Cleanup(mi.Close)

		return func() error {
			_, _, err := mi.ImportDocuments()

			return err
		}
	})
}

// writeImportFile writes the documents that the import cases load. The contents
// do not matter beyond being a valid import.
func (s *writeConcernSuite) writeImportFile() string {
	lines := make([]byte, 0, wantImportedDocs*24)
	for i := range wantImportedDocs {
		lines = append(lines, fmt.Sprintf("{\"_id\":%d,\"x\":%d}\n", i, i*i)...)
	}

	path := filepath.Join(s.T().TempDir(), "wc.json")
	s.Require().NoError(os.WriteFile(path, lines, 0o600), "can write the import file")

	return path
}

func (s *writeConcernSuite) importWithWriteConcern(
	dbName, collName, path, writeConcern string,
) error {
	mi := s.newImport(dbName, collName, path, writeConcern)
	defer mi.Close()

	imported, failed, err := mi.ImportDocuments()
	if err != nil {
		return err
	}

	// A write concern the set can satisfy has to leave the documents behind.
	// Without this an import of nothing at all would pass every succeeding case.
	s.Assert().Zero(failed, "no document fails to import")
	s.Assert().EqualValues(wantImportedDocs, imported, "every document is imported")

	return nil
}

// newImport builds the tool separately from running it, so that a test running
// the import on another goroutine can keep the steps that make assertions on the
// test's own goroutine, where testify is safe to call.
func (s *writeConcernSuite) newImport(
	dbName, collName, path, writeConcern string,
) *MongoImport {
	args := append(s.DirectToolArgs(s.PrimaryHost()),
		"--db", dbName,
		"--collection", collName,
		"--file", path,
	)
	args = sharedsuite.AppendWriteConcern(args, writeConcern)

	opts, err := ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongoimport options")

	mi, err := New(opts)
	s.Require().NoError(err, "mongoimport can connect")

	return mi
}

const wantImportedDocs = 101
