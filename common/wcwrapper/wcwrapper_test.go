package wcwrapper

import (
	"slices"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func TestMarshal(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	True, False := true, false

	cases := []struct {
		desc      string
		wc        *WriteConcern
		expectErr string // used for ErrContains
		expect    bson.D
	}{
		{
			desc:   "majority",
			wc:     Majority(),
			expect: bson.D{{"w", "majority"}},
		},
		{
			desc:   "majority with j=true",
			wc:     Wrap(&writeconcern.WriteConcern{W: "majority", Journal: &True}),
			expect: bson.D{{"w", "majority"}, {"j", true}},
		},
		{
			desc:   "w:1 with j=false",
			wc:     Wrap(&writeconcern.WriteConcern{W: 1, Journal: &False}),
			expect: bson.D{{"w", 1}, {"j", false}},
		},
		{
			desc: "w:majority with wtimeout",
			wc: &WriteConcern{
				WriteConcern: writeconcern.Majority(),
				WTimeout:     2 * time.Second,
			},
			expect: bson.D{{"w", "majority"}, {"wtimeout", int64(2000)}},
		},
		{
			desc: "custom w with j=true and wtimeout",
			wc: &WriteConcern{
				WriteConcern: &writeconcern.WriteConcern{W: "tagged", Journal: &True},
				WTimeout:     2500 * time.Millisecond,
			},
			expect: bson.D{{"w", "tagged"}, {"j", true}, {"wtimeout", int64(2500)}},
		},
		{
			desc:      "nil",
			wc:        nil,
			expectErr: "empty WriteConcern",
		},
		{
			desc:      "nil inner concern",
			wc:        &WriteConcern{},
			expectErr: "empty WriteConcern",
		},
		{
			desc:      "nil inner concern but with wtimeout",
			wc:        &WriteConcern{WTimeout: time.Second},
			expectErr: "empty WriteConcern",
		},
	}

	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			require := require.New(t)

			raw, err := bson.Marshal(test.wc)
			if test.expectErr != "" {
				require.Error(err)
				require.ErrorContains(err, test.expectErr)
				return
			}

			require.NoError(err)

			expectRaw, err := bson.Marshal(test.expect)
			require.NoError(err, "marshal expect doc to raw")
			if !slices.Equal(expectRaw, raw) {
				require.Fail(
					"marshaled write concern does not match",
					"expect: %s, got: %s",
					bson.Raw(expectRaw),
					bson.Raw(raw),
				)
			}
		})
	}

}
