package editor

import (
	"bytes"
	"io"
)

// New makes a Writer.
func New(w io.Writer, editors ...LineEditor) *Writer {
	return &Writer{
		w:       w,
		editors: editors,
	}
}

// Writer adds color to the output of an io.Writer.
type Writer struct {
	w       io.Writer
	editors []LineEditor
	buf     []byte
}

var nl = []byte{'\n'}
var cr = []byte{'\r'}

// Write implements the io.Writer interface.
func (w *Writer) Write(b []byte) (int, error) {
	lines := bytes.Split(b, nl)

	w.buf = lines[len(lines)-1]
	lines = lines[0 : len(lines)-1]

	for _, line := range lines {
		line := string(bytes.TrimSuffix(line, cr))
		for _, e := range w.editors {
			line = e.EditLine(line)
		}

		if line == "" {
			continue
		}

		_, err := io.WriteString(w.w, line+"\n")
		if err != nil {
			return 0, err
		}
	}

	return len(b), nil
}

// Flush dumps the rest of the unwritten buffer to the wrapped writer.
func (w *Writer) Flush() error {
	line := string(w.buf)
	for _, e := range w.editors {
		line = e.EditLine(line)
	}
	_, err := io.WriteString(w.w, line)
	w.buf = nil
	return err
}

// Close implements the io.Closer interface.
func (w *Writer) Close() error {
	w.buf = nil
	if wc, ok := w.w.(io.Closer); ok {
		return wc.Close()
	}

	return nil
}
