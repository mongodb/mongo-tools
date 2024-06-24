package version

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCurrent(t *testing.T) {

	origGetDate := getDate
	defer func() { getDate = origGetDate }()
	fakeDate := "20240619"
	getDate = func() string { return fakeDate }

	var (
		major             = 100
		minor             = 9
		patch             = 5
		commit            = "94783802ae607be7e3afd5cdcf2ed9853176c0ab"
		vStr              = fmt.Sprintf("%d.%d.%d", major, minor, patch)
		nonStableDescribe = fmt.Sprintf("%s-g%s", vStr, commit[:8])
		stableDescribe    = vStr
	)

	defer func() { fakeGitOutput = nil }()

	t.Run("not stable", func(t *testing.T) {
		r := require.New(t)

		fakeGitOutput = []string{commit, nonStableDescribe}

		v, err := GetCurrent()
		r.NoError(err)

		r.Equal(major, v.Major)
		r.Equal(minor, v.Minor)
		r.Equal(patch, v.Patch)
		r.Equal(commit, v.Commit)
		r.Equal(vStr, v.String())
		r.Equal(fmt.Sprintf("%s~%s.%s", vStr, fakeDate, commit[:8]), v.DebVersion())
		r.Equal(fmt.Sprintf("%s.%s", fakeDate, commit[:8]), v.RPMRelease())
	})

	t.Run("stable", func(t *testing.T) {
		r := require.New(t)

		fakeGitOutput = []string{commit, stableDescribe}

		v, err := GetCurrent()
		r.NoError(err)

		r.Equal(major, v.Major)
		r.Equal(minor, v.Minor)
		r.Equal(patch, v.Patch)
		r.Empty(v.Commit)
		r.Equal(vStr, v.String())
		r.Equal(v.String(), v.DebVersion())
		r.Equal("1", v.RPMRelease())
	})
}

func TestGreaterThan(t *testing.T) {
	r := require.New(t)

	v1, err := Parse("100.9.3")
	r.NoError(err)
	v2, err := Parse("100.9.5")
	r.NoError(err)
	v3, err := Parse("101.1.2")
	r.NoError(err)

	r.True(v3.GreaterThan(v2))
	r.True(v3.GreaterThan(v1))
	r.True(v2.GreaterThan(v1))

	r.False(v1.GreaterThan(v2))
	r.False(v1.GreaterThan(v3))
	r.False(v2.GreaterThan(v3))
}
