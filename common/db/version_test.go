package db

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
)

func TestVersionCmp(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	type testCase struct {
		v1  Version
		v2  Version
		cmp int
	}
	cases := []testCase{
		{v1: Version{0, 0, 0}, v2: Version{0, 0, 0}, cmp: 0},
		{v1: Version{0, 0, 0}, v2: Version{0, 0, 1}, cmp: -1},
		{v1: Version{0, 0, 2}, v2: Version{0, 0, 1}, cmp: 1},
		{v1: Version{0, 1, 0}, v2: Version{0, 0, 1}, cmp: 1},
		{v1: Version{0, 0, 1}, v2: Version{0, 1, 0}, cmp: -1},
		{v1: Version{0, 1, 0}, v2: Version{0, 1, 0}, cmp: 0},
		{v1: Version{0, 1, 0}, v2: Version{0, 2, 0}, cmp: -1},
		{v1: Version{0, 2, 0}, v2: Version{0, 1, 0}, cmp: 1},
		{v1: Version{1, 0, 0}, v2: Version{0, 1, 0}, cmp: 1},
		{v1: Version{0, 1, 0}, v2: Version{1, 0, 0}, cmp: -1},
		{v1: Version{1, 0, 0}, v2: Version{1, 0, 0}, cmp: 0},
		{v1: Version{2, 0, 0}, v2: Version{1, 0, 0}, cmp: 1},
		{v1: Version{1, 0, 0}, v2: Version{2, 0, 0}, cmp: -1},
	}

	for _, c := range cases {
		got := c.v1.Cmp(c.v2)
		if got != c.cmp {
			t.Errorf("%v cmp %v: got %d; wanted: %d", c.v1, c.v2, got, c.cmp)
		}
	}
}

func TestVersionComparisons(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// Expect true
	if !(Version{1, 2, 3}).LT(Version{1, 3, 0}) {
		t.Errorf("LT failed")
	}
	if !(Version{1, 3, 1}).GT(Version{1, 3, 0}) {
		t.Errorf("GT failed")
	}
	if !(Version{1, 2, 3}).LTE(Version{1, 3, 0}) {
		t.Errorf("LTE failed")
	}
	if !(Version{1, 3, 1}).GTE(Version{1, 3, 0}) {
		t.Errorf("GTE failed")
	}
	if !(Version{1, 2, 3}).LTE(Version{1, 2, 3}) {
		t.Errorf("LTE failed")
	}
	if !(Version{1, 3, 1}).GTE(Version{1, 3, 1}) {
		t.Errorf("GTE failed")
	}

	// Expect false
	if (Version{1, 2, 3}).LT(Version{1, 0, 0}) {
		t.Errorf("LT failed")
	}
	if (Version{1, 3, 1}).GT(Version{1, 3, 2}) {
		t.Errorf("GT failed")
	}
	if (Version{1, 2, 3}).LTE(Version{1, 0, 0}) {
		t.Errorf("LTE failed")
	}
	if (Version{1, 3, 1}).GTE(Version{1, 3, 2}) {
		t.Errorf("GTE failed")
	}
	if (Version{1, 2, 3}).LTE(Version{1, 2, 2}) {
		t.Errorf("LTE failed")
	}
	if (Version{1, 3, 1}).GTE(Version{1, 3, 2}) {
		t.Errorf("GTE failed")
	}
}
