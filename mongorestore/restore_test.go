// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"fmt"
	"slices"
	"testing"

	"github.com/mongodb/mongo-tools/common/idx"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
)

func Test_removeDefaultIdIndex(t *testing.T) {
	cases := []struct {
		Label            string
		Input            []*idx.IndexDocument
		DefaultIdIndexAt int
	}{
		{
			Label: "single legacy index",
			Input: []*idx.IndexDocument{
				{
					Key: bson.D{{"_id", ""}},
				},
			},
			DefaultIdIndexAt: 0,
		},
	}

	for _, curCase := range cases {
		Convey(
			fmt.Sprintf("Verfifying that default _id indexes are removed when needed: %s", curCase.Label),
			t,
			func() {
				expect := slices.Clone(curCase.Input)
				if curCase.DefaultIdIndexAt >= 0 {
					expect = slices.Delete(expect, curCase.DefaultIdIndexAt, 1+curCase.DefaultIdIndexAt)
				}

				got, err := removeDefaultIdIndex(slices.Clone(curCase.Input))
				So(err, ShouldBeNil)
				So(got, ShouldEqual, expect)
			},
		)
	}
}
