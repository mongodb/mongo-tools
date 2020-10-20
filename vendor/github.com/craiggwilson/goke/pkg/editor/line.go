package editor

import (
	"regexp"
)

// LineEditor adds color to a line.
type LineEditor interface {
	EditLine(string) string
}

// LineEditorFunc is a function implementation of LineEditor.
type LineEditorFunc func(string) string

// EditLine implements the LineEditor interface.
func (f LineEditorFunc) EditLine(line string) string {
	return f(line)
}

// Replace a line that matches a pattern.
func Replace(pattern string, editor LineEditor) LineEditor {
	p := regexp.MustCompile(pattern)
	return LineEditorFunc(func(line string) string {
		if !p.MatchString(line) {
			return line
		}

		return editor.EditLine(line)
	})
}

// Remove a line from the output that matches a pattern.
func Remove(pattern string) LineEditor {
	p := regexp.MustCompile(pattern)
	return LineEditorFunc(func(line string) string {
		if !p.MatchString(line) {
			return line
		}

		return ""
	})
}
