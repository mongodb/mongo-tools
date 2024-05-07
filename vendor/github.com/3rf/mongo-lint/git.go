package lint

import (
	"fmt"
	"os/exec"
	"strings"
)

func GitBlameFileAtLine(filename string, line int) string {
	cmd := exec.Command("git", "blame", filename, "-L",
		fmt.Sprintf("%v,%v", line, line), "-e")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	blame := string(output)
	cutoff := strings.Index(blame, ")")
	if cutoff == -1 {
		return ""
	}
	return blame[:cutoff+1]
}
