package version

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

	v.Commit = commit
	return v, nil
}

func GetFromRev(rev string) (Version, error) {
	commit, err := git("rev-parse", rev)
	if err != nil {
		return Version{}, fmt.Errorf("git rev-parse %s failed: %w", rev, err)
	}

	desc, err := git("describe", commit)
	if err != nil {
		return Version{}, fmt.Errorf("git describe %s failed: %w", commit, err)
	}

	v, err := Parse(desc)
	if err != nil {
		return Version{}, fmt.Errorf("failed to parse version from describe: %w", err)
	}

	v.Commit = commit
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
	return strings.Split(v.Pre, "-")[0]
}

func (v Version) IsStable() bool {
	return v.Pre == ""
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
