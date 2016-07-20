package stat_consumer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/mongodb/mongo-tools/common/text"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer/line"
	"github.com/nsf/termbox-go"
)

// A LineFormatter formats StatLines for printing.
type LineFormatter interface {
	// FormatLines returns the string representation of the StatLines that are passed in.
	FormatLines(lines []*line.StatLine, headerKeys []string, keyNames map[string]string) string

	// IsFinished returns true iff the formatter cannot print any more data
	IsFinished() bool
}

type limitableFormatter struct {
	// atomic operations are performed on rowCount, so these two variables
	// should stay at the beginning for the sake of variable alignment
	maxRows, rowCount int64
}

func (lf *limitableFormatter) increment() {
	atomic.AddInt64(&lf.rowCount, 1)
}

func (lf *limitableFormatter) IsFinished() bool {
	return lf.maxRows > 0 && atomic.LoadInt64(&lf.rowCount) >= lf.maxRows
}

// JSONLineFormatter converts the StatLines to JSON
type JSONLineFormatter struct {
	*limitableFormatter
}

func NewJSONLineFormatter(maxRows int64) *JSONLineFormatter {
	return &JSONLineFormatter{
		limitableFormatter: &limitableFormatter{maxRows: maxRows},
	}
}

// FormatLines formats the StatLines as JSON
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

	jlf.increment()
	return fmt.Sprintf("%s\n", linesAsJsonBytes)
}

// GridLineFormatter uses a text.GridWriter to format the StatLines as a grid
type GridLineFormatter struct {
	*limitableFormatter
	*text.GridWriter

	// If true, enables printing of headers to output
	includeHeader bool

	// Counter for periodic headers
	index int

	// Tracks number of hosts so we can reprint headers when it changes
	prevLineCount int
}

func NewGridLineFormatter(maxRows int64, includeHeader bool) *GridLineFormatter {
	return &GridLineFormatter{
		limitableFormatter: &limitableFormatter{maxRows: maxRows},
		includeHeader:      includeHeader,
		GridWriter:         &text.GridWriter{ColumnPadding: 1},
	}
}

// headerInterval is the number of chunks before the header is re-printed in GridLineFormatter
const headerInterval = 10

// FormatLines formats the StatLines as a grid
func (glf *GridLineFormatter) FormatLines(lines []*line.StatLine, headerKeys []string, keyNames map[string]string) string {
	buf := &bytes.Buffer{}

	// Sort the stat lines by hostname, so that we see the output
	// in the same order for each snapshot
	sort.Sort(line.StatLines(lines))

	// Print the columns that are enabled
	for _, key := range headerKeys {
		header := keyNames[key]
		glf.WriteCell(header)
	}
	glf.EndRow()

	for _, l := range lines {
		if l.Printed && l.Error == nil {
			l.Error = fmt.Errorf("no data received")
		}
		l.Printed = true

		if l.Error != nil {
			glf.WriteCell(l.Fields["host"])
			glf.Feed(l.Error.Error())
			continue
		}

		for _, key := range headerKeys {
			glf.WriteCell(l.Fields[key])
		}
		glf.EndRow()
	}
	glf.Flush(buf)

	// clear the flushed data
	glf.Reset()

	gridLine := buf.String()

	if glf.prevLineCount != len(lines) {
		glf.index = 0
	}
	glf.prevLineCount = len(lines)

	if !glf.includeHeader || glf.index != 0 {
		// Strip out the first line of the formatted output,
		// which contains the headers. They've been left in up until this point
		// in order to force the formatting of the columns to be wide enough.
		firstNewLinePos := strings.Index(gridLine, "\n")
		if firstNewLinePos >= 0 {
			gridLine = gridLine[firstNewLinePos+1:]
		}
	}
	glf.index++
	if glf.index == headerInterval {
		glf.index = 0
	}

	if len(lines) > 1 {
		// For multi-node stats, add an extra newline to tell each block apart
		gridLine = fmt.Sprintf("\n%s", gridLine)
	}
	glf.increment()
	return gridLine
}

// InteractiveLineFormatter produces ncurses-style output
type InteractiveLineFormatter struct {
	*limitableFormatter

	includeHeader bool
	table         []*column
	row, col      int
	showHelp      bool
}

func NewInteractiveLineFormatter(includeHeader bool) *InteractiveLineFormatter {
	ilf := &InteractiveLineFormatter{
		limitableFormatter: &limitableFormatter{maxRows: 1},
		includeHeader:      includeHeader,
	}
	if err := termbox.Init(); err != nil {
		fmt.Printf("Error setting up terminal UI: %v", err)
		panic("could not set up interactive terminal interface")
	}
	go func() {
		for {
			ilf.handleEvent(termbox.PollEvent())
			ilf.update()
		}
	}()
	return ilf
}

type column struct {
	cells []*cell
	width int
}

type cell struct {
	text     string
	changed  bool
	feed     bool
	selected bool
	header   bool
}

// FormatLines formats the StatLines as a table in the terminal ui
func (ilf *InteractiveLineFormatter) FormatLines(lines []*line.StatLine, headerKeys []string, keyNames map[string]string) string {
	// keep ordering consistent
	sort.Sort(line.StatLines(lines))

	if ilf.includeHeader {
		headerLine := &line.StatLine{
			Fields: keyNames,
		}
		lines = append([]*line.StatLine{headerLine}, lines...)
	}

	// add new rows and columns when new hosts and stats are shown
	for len(ilf.table) < len(headerKeys) {
		ilf.table = append(ilf.table, new(column))
	}
	for _, column := range ilf.table {
		for len(column.cells) < len(lines) {
			column.cells = append(column.cells, new(cell))
		}
	}

	for i, column := range ilf.table {
		key := headerKeys[i]
		for j, cell := range column.cells {
			// i, j <=> col, row
			l := lines[j]
			if l.Error != nil && i == 0 {
				cell.text = fmt.Sprintf("%s: %s", l.Fields["host"], l.Error)
				cell.feed = true
				continue
			}
			newText := l.Fields[key]
			cell.changed = cell.text != newText
			cell.text = newText
			cell.feed = false
			cell.header = j == 0 && ilf.includeHeader
			if w := len(cell.text); w > column.width {
				column.width = w
			}
		}
	}

	ilf.update()
	return ""
}

func (ilf *InteractiveLineFormatter) handleEvent(ev termbox.Event) {
	if ev.Type != termbox.EventKey {
		return
	}
	currSelected := ilf.table[ilf.col].cells[ilf.row].selected
	switch {
	case ev.Key == termbox.KeyCtrlC:
		fallthrough
	case ev.Key == termbox.KeyEsc:
		fallthrough
	case ev.Ch == 'q':
		termbox.Close()
		// our max rowCount is set to 1; increment to exit
		ilf.increment()
	case ev.Key == termbox.KeyArrowRight:
		fallthrough
	case ev.Ch == 'l':
		if ilf.col+1 < len(ilf.table) {
			ilf.col++
		}
	case ev.Key == termbox.KeyArrowLeft:
		fallthrough
	case ev.Ch == 'h':
		if ilf.col > 0 {
			ilf.col--
		}
	case ev.Key == termbox.KeyArrowDown:
		fallthrough
	case ev.Ch == 'j':
		if ilf.row+1 < len(ilf.table[0].cells) {
			ilf.row++
		}
	case ev.Key == termbox.KeyArrowUp:
		fallthrough
	case ev.Ch == 'k':
		if ilf.row > 0 {
			ilf.row--
		}
	case ev.Ch == 's':
		cell := ilf.table[ilf.col].cells[ilf.row]
		cell.selected = !cell.selected
	case ev.Key == termbox.KeySpace:
		for _, column := range ilf.table {
			for _, cell := range column.cells {
				cell.selected = false
			}
		}
	case ev.Ch == 'c':
		for _, cell := range ilf.table[ilf.col].cells {
			cell.selected = !currSelected
		}
	case ev.Ch == 'v':
		for _, column := range ilf.table {
			cell := column.cells[ilf.row]
			cell.selected = !currSelected
		}
	case ev.Ch == 'r':
		termbox.Sync()
	case ev.Ch == '?':
		ilf.showHelp = !ilf.showHelp
	default:
		// ouput a bell on unknown inputs
		fmt.Printf("\a")
	}
}

const (
	helpPrompt  = `Press '?' to toggle help`
	helpMessage = `
Exit: 'q' or <Esc>
Navigation: arrow keys or 'h', 'j', 'k', and 'l'
Highlighting: 'v' to toggle row
              'c' to toggle column
              's' to toggle cell
              <Space> to clear all highlighting
Redraw: 'r' to fix broken-looking output`
)

func writeString(x, y int, text string, fg, bg termbox.Attribute) {
	for i, str := range strings.Split(text, "\n") {
		for j, ch := range str {
			termbox.SetCell(x+j, y+i, ch, fg, bg)
		}
	}
}

func (ilf *InteractiveLineFormatter) update() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	x := 0
	for i, column := range ilf.table {
		for j, cell := range column.cells {
			if ilf.col == i && ilf.row == j {
				termbox.SetCursor(x+column.width-1, j)
			}
			if cell.text == "" {
				continue
			}
			fgAttr := termbox.ColorWhite
			bgAttr := termbox.ColorDefault
			if cell.selected {
				fgAttr = termbox.ColorBlack
				bgAttr = termbox.ColorWhite
			}
			if cell.changed || cell.feed {
				fgAttr |= termbox.AttrBold
			}
			if cell.header {
				fgAttr |= termbox.AttrUnderline
				fgAttr |= termbox.AttrBold
			}
			padding := column.width - len(cell.text)
			if cell.feed && padding < 0 {
				padding = 0
			}
			writeString(x, j, strings.Repeat(" ", padding), termbox.ColorDefault, bgAttr)
			writeString(x+padding, j, cell.text, fgAttr, bgAttr)
		}
		x += 1 + column.width
	}
	rowCount := len(ilf.table[0].cells)
	writeString(0, rowCount+1, helpPrompt, termbox.ColorWhite, termbox.ColorDefault)
	if ilf.showHelp {
		writeString(0, rowCount+2, helpMessage, termbox.ColorWhite, termbox.ColorDefault)
	}
	termbox.Flush()
}
