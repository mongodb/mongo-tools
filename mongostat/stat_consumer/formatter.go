package stat_consumer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mongodb/mongo-tools/common/text"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer/line"
)

// A LineFormatter formats StatLines for printing.
type LineFormatter interface {
	// FormatLines returns the string representation of the StatLines that are passed in.
	FormatLines(lines []*line.StatLine, headerKeys []string, keyNames map[string]string) string
}

// Implementation of LineFormatter - converts the StatLines to JSON.
type JSONLineFormatter struct{}

// Satisfy the LineFormatter interface. Formats the StatLines as JSON.
func (jlf *JSONLineFormatter) FormatLines(lines []*line.StatLine, headerKeys []string, keyNames map[string]string) string {
	// middle ground b/t the StatLines and the JSON string to be returned
	jsonFormat := map[string]interface{}{}

	// convert each StatLine to JSON
	for _, l := range lines {
		lineJson := make(map[string]interface{})

		if l.Printed && l.Error == nil {
			l.Error = fmt.Errorf("no data received")
		}
		l.Printed = true

		// check for error
		if l.Error != nil {
			lineJson["error"] = l.Error.Error()
			jsonFormat[l.Fields["host"]] = lineJson
			continue
		}

		for _, key := range headerKeys {
			lineJson[keyNames[key]] = l.Fields[key]
		}
		jsonFormat[l.Fields["host"]] = lineJson
	}

	// convert the JSON format of the lines to a json string to be returned
	linesAsJsonBytes, err := json.Marshal(jsonFormat)
	if err != nil {
		return fmt.Sprintf(`{"json error": "%v"}`, err.Error())
	}

	return fmt.Sprintf("%s\n", linesAsJsonBytes)
}

// Implementation of LineFormatter - uses a common/text.GridWriter to format
// the StatLines as a grid.
type GridLineFormatter struct {
	// If true, enables printing of headers to output
	IncludeHeader bool

	// Number of line outputs to skip between adding in headers
	HeaderInterval int

	// Grid writer
	Writer *text.GridWriter

	// Counter for periodic headers
	index int

	// Tracks number of hosts so we can reprint headers when it changes
	prevLineCount int
}

// Satisfy the LineFormatter interface. Formats the StatLines as a grid.
func (glf *GridLineFormatter) FormatLines(lines []*line.StatLine, headerKeys []string, keyNames map[string]string) string {
	buf := &bytes.Buffer{}

	// Sort the stat lines by hostname, so that we see the output
	// in the same order for each snapshot
	sort.Sort(line.StatLines(lines))

	// Print the columns that are enabled
	for _, key := range headerKeys {
		header := keyNames[key]
		glf.Writer.WriteCell(header)
	}
	glf.Writer.EndRow()

	for _, l := range lines {
		if l.Printed && l.Error == nil {
			l.Error = fmt.Errorf("no data received")
		}
		l.Printed = true

		if l.Error != nil {
			glf.Writer.WriteCell(l.Fields["host"])
			glf.Writer.Feed(l.Error.Error())
			continue
		}

		// Write the opcount columns (always active)
		for _, key := range headerKeys {
			glf.Writer.WriteCell(l.Fields[key])
		}
		glf.Writer.EndRow()
	}
	glf.Writer.Flush(buf)

	// clear the flushed data
	glf.Writer.Reset()

	gridLine := buf.String()

	if glf.prevLineCount != len(lines) {
		glf.index = 0
	}
	glf.prevLineCount = len(lines)

	if !glf.IncludeHeader || glf.index != 0 {
		// Strip out the first line of the formatted output,
		// which contains the headers. They've been left in up until this point
		// in order to force the formatting of the columns to be wide enough.
		firstNewLinePos := strings.Index(gridLine, "\n")
		if firstNewLinePos >= 0 {
			gridLine = gridLine[firstNewLinePos+1:]
		}
	}
	glf.index++
	if glf.index == glf.HeaderInterval {
		glf.index = 0
	}

	if len(lines) > 1 {
		// For multi-node stats, add an extra newline to tell each block apart
		gridLine = fmt.Sprintf("\n%s", gridLine)
	}
	return gridLine
}
