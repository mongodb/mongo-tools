package task

import (
	"github.com/mgutz/ansi"
)

func newTUI(useColors bool) *TUI {
	if !useColors {
		return nil
	}

	return &TUI{}
}

// TUI is responsible for setting coloring information.
type TUI struct{}

// Error colors the msg as an error.
func (ui *TUI) Error(msg string) string {
	if ui == nil {
		return msg
	}

	return ansi.Color(msg, "red+b")
}

// Highlight colors the msg as a highlight.
func (ui *TUI) Highlight(msg string) string {
	if ui == nil {
		return msg
	}

	return ansi.Color(msg, "white+bh")
}

// Info colors the msg as information.
func (ui *TUI) Info(msg string) string {
	if ui == nil {
		return msg
	}

	return ansi.Color(msg, "cyan+b")
}

// Lowlight colors the msg as a lowlight.
func (ui *TUI) Lowlight(msg string) string {
	if ui == nil {
		return msg
	}

	return ansi.Color(msg, "black+bh")
}

// Success colors the msg as success.
func (ui *TUI) Success(msg string) string {
	if ui == nil {
		return msg
	}

	return ansi.Color(msg, "green+b")
}
