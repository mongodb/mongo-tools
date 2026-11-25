// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build integration || sharded

package testutil

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestEnsureToolUsers checks that EnsureBackupUser and EnsureRestoreUser create their users
// with exactly the one role the documentation prescribes for their tool, which is what every
// helper that authenticates a tool as one of them relies on.
func TestEnsureToolUsers(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	cases := []struct {
		username string
		ensure   func(*testing.T)
		role     string
	}{
		{testopts.BackupUsername, EnsureBackupUser, "backup"},
		{testopts.RestoreUsername, EnsureRestoreUser, "restore"},
	}

	for _, testCase := range cases {
		t.Run(testCase.username, func(t *testing.T) {
			testCase.ensure(t)

			session, err := GetBareSession(t)
			require.NoError(t, err, "connecting as the harness user")

			var info struct {
				Users []struct {
					User  string `bson:"user"`
					DB    string `bson:"db"`
					Roles []struct {
						Role string `bson:"role"`
						DB   string `bson:"db"`
					} `bson:"roles"`
				} `bson:"users"`
			}
			err = session.Database("admin").RunCommand(t.Context(), bson.D{
				{"usersInfo", []bson.D{{{"user", testCase.username}, {"db", "admin"}}}},
			}).Decode(&info)
			require.NoError(t, err, "looking up the tool user after the ensure call")

			require.Len(t, info.Users, 1, "the tool user exists after the ensure call")
			user := info.Users[0]
			assert.Equal(t, testCase.username, user.User, "the tool user's name")
			assert.Equal(t, "admin", user.DB, "the tool user is created on the admin database")

			require.Len(t, user.Roles, 1, "the tool user holds exactly its tool's one role")
			assert.Equal(t, testCase.role, user.Roles[0].Role, "the tool user's single role")
			assert.Equal(
				t,
				"admin",
				user.Roles[0].DB,
				"the tool user's role comes from the admin database",
			)
		})
	}
}
