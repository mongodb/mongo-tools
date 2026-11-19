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

// TestEnsureBackupRestoreUser checks that EnsureBackupRestoreUser creates the tool user with
// exactly the backup and restore roles, which is what every helper that authenticates a tool as
// testopts.ToolUsername relies on.
func TestEnsureBackupRestoreUser(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	EnsureBackupRestoreUser(t)

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
		{Key: "usersInfo", Value: bson.A{bson.D{
			{Key: "user", Value: testopts.ToolUsername},
			{Key: "db", Value: "admin"},
		}}},
	}).Decode(&info)
	require.NoError(t, err, "looking up the tool user after EnsureBackupRestoreUser")

	require.Len(t, info.Users, 1, "the tool user exists after EnsureBackupRestoreUser")
	user := info.Users[0]
	assert.Equal(t, testopts.ToolUsername, user.User, "the tool user's name")
	assert.Equal(t, "admin", user.DB, "the tool user is created on the admin database")

	assert.Len(t, user.Roles, 2, "the tool user holds exactly the two documented roles")
	roleNames := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		assert.Equal(t, "admin", role.DB, "the tool user's roles come from the admin database")
		roleNames = append(roleNames, role.Role)
	}
	assert.ElementsMatch(
		t,
		[]string{"backup", "restore"},
		roleNames,
		"the tool user holds the backup and restore roles and nothing else",
	)
}
