package version

import (
	"fmt"
	"os/exec"
	"strings"
)

type Version struct {
	Commit   string
	Describe string
	IsStable bool
}

func (v Version) String() string {
	return v.Describe
}

func GetCurrent() (Version, error) {
	commit, err := git("rev-parse", "HEAD")
	if err != nil {
		return Version{}, err
	}

	_, err = git("describe", "--exact")
	isTagged := err == nil

	desc, err := git("describe", "--dirty")
	if err != nil {
		return Version{}, err
	}

	ver := Version{
		Commit:   commit,
		Describe: desc,
		IsStable: isTagged,
	}

	return ver, nil
}

func GetFromRev(rev string) (Version, error) {
	commit, err := git("rev-parse", rev)
	if err != nil {
		return Version{}, err
	}

	_, err = git("describe", "--exact", commit)
	isTagged := err == nil

	desc, err := git("describe", commit)
	if err != nil {
		return Version{}, err
	}

	ver := Version{
		Commit:   commit,
		Describe: desc,
		IsStable: isTagged,
	}

	return ver, nil
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
