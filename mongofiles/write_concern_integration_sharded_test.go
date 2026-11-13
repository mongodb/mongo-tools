// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build integration || sharded

package mongofiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type writeConcernSuite struct {
	sharedsuite.IntegrationSuite
}

// TestMongoFilesWriteConcern checks that mongofiles succeeds or fails according
// to whether the replica set can satisfy the write concern it was given, and
// that without a timeout it waits for the secondaries rather than giving up.
func TestMongoFilesWriteConcern(t *testing.T) {
	suite.Run(t, new(writeConcernSuite))
}

// TestSatisfiability is mongoimport's write-concern matrix for mongofiles: it
// writes through GridFS rather than an insert, so the write concern reaches the
// server through different code.
func (s *writeConcernSuite) TestSatisfiability() {
	testutil.SkipUnlessReplicaSet(s.T())

	dbName := s.DBName()
	putFile := s.writePutFile()

	s.RunWriteConcernCases(dbName, func(writeConcern string) error {
		return s.putWithWriteConcern(dbName, putFile, writeConcern)
	})
}

// TestWaitsForSlowReplication is the import's waiting test for mongofiles. It
// writes through GridFS rather than an insert, so the waiting is not the same
// code as the import's.
func (s *writeConcernSuite) TestWaitsForSlowReplication() {
	testutil.SkipUnlessReplicaSet(s.T())

	dbName := s.DBName()
	putFile := s.writePutFile()

	s.AssertWaitsForReplication("mongofiles", func(writeConcern string) func() error {
		mf := s.newPut(dbName, putFile, writeConcern)
		s.T().Cleanup(mf.Close)

		return func() error {
			_, err := mf.Run(false)

			return err
		}
	})
}

func (s *writeConcernSuite) writePutFile() string {
	path := filepath.Join(s.T().TempDir(), "wc.txt")
	s.Require().NoError(
		os.WriteFile(path, []byte("mongofiles write concern test data\n"), 0o600),
		"can write the file that mongofiles puts",
	)

	return path
}

func (s *writeConcernSuite) putWithWriteConcern(dbName, path, writeConcern string) error {
	mf := s.newPut(dbName, path, writeConcern)
	defer mf.Close()

	if _, err := mf.Run(false); err != nil {
		return err
	}

	// As with the import, a write concern the set can satisfy has to leave the
	// file behind, so that storing nothing cannot pass as a success. mongofiles
	// stores the path it was given as the file's name, rather than just the base
	// name.
	stored, err := s.Client().Database(dbName).
		Collection(gridFSFilesCollection).
		CountDocuments(s.Context(), bson.D{{"filename", path}})
	s.Assert().NoError(err, "can count the files mongofiles stored")
	s.Assert().EqualValues(1, stored, "the file mongofiles put is stored")

	return nil
}

const gridFSFilesCollection = "fs.files"

// newPut builds the tool separately from running it, so that a test running the
// put on another goroutine can keep the steps that make assertions on the test's
// own goroutine, where testify is safe to call.
func (s *writeConcernSuite) newPut(dbName, path, writeConcern string) *MongoFiles {
	args := append(s.DirectToolArgs(s.PrimaryHost()), "--db", dbName)
	args = sharedsuite.AppendWriteConcern(args, writeConcern)
	args = append(args, "put", path)

	opts, err := ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongofiles options")

	mf, err := New(opts)
	s.Require().NoError(err, "mongofiles can connect")

	return mf
}
