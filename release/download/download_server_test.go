package download

import (
	"testing"

	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/stretchr/testify/assert"
)

func testFeed() *ServerJSONFeed {
	serverVersion := func(v string, targets ...string) *ServerVersion {
		sv := &ServerVersion{Version: v, GitHash: "hash-" + v}
		for _, t := range targets {
			sv.Downloads = append(sv.Downloads, &ServerDownload{
				Target:  t,
				Edition: "enterprise",
				Arch:    "x86_64",
				Archive: ServerArchive{URL: "https://example.com/" + v + ".tgz"},
			})
		}
		return sv
	}

	return &ServerJSONFeed{
		Versions: []*ServerVersion{
			serverVersion("7.1.0-alpha0", "rhel8"),
			serverVersion("7.0.41-rc0", "rhel8"),
			serverVersion("7.0.40", "rhel9"),
			serverVersion("7.0.39", "rhel8"),
			serverVersion("7.0.38", "rhel8"),
			serverVersion("6.0.20", "rhel8"),
		},
	}
}

func testPlatform() platform.Platform {
	return platform.Platform{
		Name: "rhel8",
		Arch: platform.ArchX86_64,
	}
}

func TestCandidateVersions(t *testing.T) {
	feed := testFeed()
	pf := testPlatform()

	t.Run("skips versions without a matching download", func(t *testing.T) {
		// 7.0.40 only publishes an rhel9 archive, and 7.0.41-rc0 is a release candidate.
		got := feed.CandidateVersions("7.0.39", pf, "enterprise", 5)
		assert.Equal(t, []string{"7.0.39", "7.0.38"}, got)
	})

	t.Run("respects the limit", func(t *testing.T) {
		got := feed.CandidateVersions("7.0.39", pf, "enterprise", 1)
		assert.Equal(t, []string{"7.0.39"}, got)
	})

	t.Run("only matches the same major.minor", func(t *testing.T) {
		got := feed.CandidateVersions("6.0.20", pf, "enterprise", 5)
		assert.Equal(t, []string{"6.0.20"}, got)
	})

	t.Run("includes the requested release candidate", func(t *testing.T) {
		got := feed.CandidateVersions("7.0.41-rc0", pf, "enterprise", 5)
		assert.Equal(t, []string{"7.0.41-rc0", "7.0.39", "7.0.38"}, got)
	})

	t.Run("no match for a different edition", func(t *testing.T) {
		got := feed.CandidateVersions("7.0.39", pf, "targeted", 5)
		assert.Empty(t, got)
	})

	t.Run("first candidate agrees with FindURLHashAndVersion", func(t *testing.T) {
		_, _, version, err := feed.FindURLHashAndVersion("7.0.39", pf, "enterprise")
		assert.NoError(t, err)

		candidates := feed.CandidateVersions("7.0.39", pf, "enterprise", 5)
		assert.NotEmpty(t, candidates)
		assert.Equal(t, version, candidates[0])
	})
}
