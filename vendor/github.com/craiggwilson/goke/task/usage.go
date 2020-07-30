package task

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func usage(ui *TUI, fs *flag.FlagSet, registry *Registry) {
	var buf bytes.Buffer
	usageTemp(ui, fs, registry, 0, &buf)
	rd := bufio.NewReader(&buf)
	maxLine := 0
	for {
		line, _, err := rd.ReadLine()
		if len(line) > maxLine {
			maxLine = len(line)
			if maxLine > 80 {
				maxLine = 80
			}
		}

		if err != nil {
			break
		}
	}

	usageTemp(ui, fs, registry, maxLine, os.Stdout)
}

func usageTemp(ui *TUI, fs *flag.FlagSet, registry *Registry, longestLine int, out io.Writer) {
	fmt.Fprintln(out, ui.Highlight("USAGE")+": [tasks ...] [options ...]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, ui.Highlight("TASKS")+":")
	currentNS := ""
	for i, t := range registry.Tasks() {
		if t.Hidden() {
			continue
		}
		taskNS := registry.taskNamespace(t)
		if taskNS == "" {
			taskNS = t.Name()
		}
		if currentNS == "" || !strings.HasPrefix(taskNS, currentNS) {
			if i != 0 {
				fmt.Fprintln(out, ui.Lowlight("  "+strings.Repeat("-", longestLine)))
			}
			currentNS = taskNS
		}
		fmt.Fprint(out, "  ", ui.Info(t.Name()))
		args := t.DeclaredArgs()
		if len(args) > 0 {
			fmt.Fprint(out, "(")
			for i, a := range args {
				fmt.Fprint(out, a.Name)
				if i < len(args)-1 {
					fmt.Fprint(out, ", ")
				}
			}
			fmt.Fprint(out, ")")
		}
		if len(t.Dependencies()) > 0 {
			fmt.Fprint(out, " -> ", t.Dependencies())
		}
		fmt.Fprintln(out)
		if t.Description() != "" {
			fmt.Fprintln(out, "       ", t.Description())
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "OPTIONS:")
	fs.SetOutput(out)
	fs.PrintDefaults()
}
