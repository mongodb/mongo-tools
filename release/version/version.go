package version

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Version struct {
	Major int
	Minor int
	Patch int

	Commit string
}

func Parse(desc string) (Version, error) {
	// This throws out the part after the dash, but if this was called from
	// `GetCurrent`, then we'll capture the commit in the `Commit` field in
	// that func.
	parts := strings.Split(strings.Split(desc, "-")[0], ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, fmt.Errorf(
			"could not find a two- or three-part dotted version in %q",
			desc,
		)
	}

	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("failed to parse major version %q", parts[0])
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("failed to parse minor version %q", parts[1])
	}

	var patch int
	if len(parts) == 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse patch version %q", parts[2])
		}
	}

	return Version{
		Major: maj,
		Minor: minor,
		Patch: patch,
	}, nil
}

var tagRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func GetCurrent() (Version, error) {
	commit, err := git("rev-parse", "HEAD")
	if err != nil {
		return Version{}, fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}

	desc, err := git("describe")
	if err != nil {
		return Version{}, fmt.Errorf("git describe failed: %w", err)
	}

	v, err := Parse(desc)
	if err != nil {
		return Version{}, fmt.Errorf("failed to parse version from describe: %w", err)
	}

	if !tagRE.MatchString(desc) {
		v.Commit = commit
	}

	return v, nil
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) StringWithCommit() string {
	if v.Commit == "" {
		return v.String()
	}
	// The "g" before the commit is to imitate the output of "git describe",
	// which gives us something like "100.9.5-4-gab7ebc58".
	return fmt.Sprintf("%s-g%s", v.String(), v.Commit[:8])
}

// This is a function so we can replace this in test code.
var getDate = func() string {
	return time.Now().Format("20060102")
}

func (v Version) DebVersion() string {
	if v.Commit == "" {
		return v.String()
	}

	return fmt.Sprintf("%s~%s.%s", v.String(), getDate(), v.Commit[:8])
}

func (v Version) RPMRelease() string {
	if v.Commit != "" {
		return fmt.Sprintf("%s.%s", getDate(), v.Commit[:8])
	}
	return "1"
}

func (v Version) IsStable() bool {
	return v.Commit == ""
}

func (v Version) GreaterThan(other Version) bool {
	if v.Major > other.Major {
		return true
	} else if v.Major < other.Major {
		return false
	}

	if v.Minor > other.Minor {
		return true
	} else if v.Minor < other.Minor {
		return false
	}

	return v.Patch > other.Patch
}

// This exists to make it possible to test code which calls `git`.
var fakeGitOutput []string

func git(args ...string) (string, error) {
	if len(fakeGitOutput) > 0 {
		output := fakeGitOutput[0]
		fakeGitOutput = fakeGitOutput[1:]
		return output, nil
	}

	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		if exerr, ok := err.(*exec.ExitError); ok {
			err = fmt.Errorf("ExitError: %v. Stderr: %q", err, string(exerr.Stderr))
		}
	}
	return strings.TrimSpace(string(out)), err
}
