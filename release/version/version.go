package version

import (
	"fmt"
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
	Pre   string

	Commit string
}

func Parse(desc string) (Version, error) {
	if desc[0] == 'r' || desc[0] == 'v' {
		desc = desc[1:]
	}

	parts := strings.SplitN(desc, "-", 2)

	num := parts[0]
	pre := ""
	if len(parts) > 1 {
		pre = parts[1]
	}

	parts = strings.Split(num, ".")

	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("failed to parse major version %q", parts[0])
	}

	minor := 0
	if len(parts) > 1 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse minor version %q", parts[1])
		}
	}

	pat := 0
	if len(parts) > 2 {
		pat, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse patch version %q", parts[2])
		}
	}

	return Version{
		Major: maj,
		Minor: minor,
		Patch: pat,
		Pre:   pre,
	}, nil
}

var tagRE = regexp.MustCompile(`^\d+\.\d+\.d+$`)

func GetCurrent() (Version, error) {
	commit, err := git("rev-parse", "HEAD")
	if err != nil {
		return Version{}, fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}

	desc, err := git("describe", "--dirty")
	if err != nil {
		return Version{}, fmt.Errorf("git describe --dirty failed: %w", err)
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
	vStr := v.StringWithoutPre()
	if v.Pre != "" {
		vStr = fmt.Sprintf("%s-%s", vStr, v.Pre)
	}
	return vStr
}

func (v Version) StringWithoutPre() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) RPMRelease() string {
	if v.Pre == "" {
		return "1"
	}
	pre := v.Pre
	if v.Commit != "" {
		pre = time.Now().Format("20060102") + "." + v.Commit[:8]
	}
	return pre
}

func (v Version) IsStable() bool {
	return v.Pre == ""
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

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exerr, ok := err.(*exec.ExitError); ok {
			err = fmt.Errorf("ExitError: %v. Stderr: %q", err, string(exerr.Stderr))
		}
	}
	return strings.TrimSpace(string(out)), err
}
