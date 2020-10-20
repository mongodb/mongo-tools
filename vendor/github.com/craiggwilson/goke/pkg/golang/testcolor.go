package golang

import (
	"io"

	"github.com/craiggwilson/goke/pkg/editor"

	"github.com/mgutz/ansi"
)

func ColoredTestWriter(w io.Writer) *editor.Writer {
	return editor.New(
		w,
		editor.Replace(`(?m)^ok.*`, editor.LineEditorFunc(ansi.ColorFunc("green+b"))),
		editor.Replace(`(?m)^PASS.*`, editor.LineEditorFunc(ansi.ColorFunc("green+b"))),
		editor.Replace(`(?m)^(\s*)--- PASS.*`, editor.LineEditorFunc(ansi.ColorFunc("green+b"))),
		editor.Replace(`(?m)^\?.*`, editor.LineEditorFunc(ansi.ColorFunc("grey+b"))),
		editor.Replace(`(?m)^FAIL.*`, editor.LineEditorFunc(ansi.ColorFunc("red+b"))),
		editor.Replace(`(?m)^(\s*)--- FAIL:.*`, editor.LineEditorFunc(ansi.ColorFunc("red+b"))),
		editor.Remove(`(?m)^(\s*)=== .*`),
	)
}
