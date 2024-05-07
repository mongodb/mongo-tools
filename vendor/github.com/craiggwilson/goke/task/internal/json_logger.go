package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	startTimeNanosField = "startTimeNanos"
	msgField            = "msg"
)

type JSONLogger struct {
	w  io.Writer
	id string
}

// NewJSONLogger creates a new JSONLogger which generates valid JSON ouputs and includes the same unique
// id in each log line generated through this logger.
func NewJSONLogger(w io.Writer) *JSONLogger {
	return &JSONLogger{
		w: w,
		// Use nano timestamps as a decent unique identifier that does not
		// require any external dependencies.
		id: fmt.Sprintf("%d", time.Now().UnixNano()),
	}
}

func (j *JSONLogger) Logln(msg string, fields map[string]string) {
	_, _ = j.logln(msg, fields)
}

func (j *JSONLogger) logln(msg string, fields map[string]string) (int, error) {
	fields[msgField] = msg
	fields[startTimeNanosField] = j.id
	logString, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return fmt.Fprintln(j.w, string(logString))
}

// Write implements the [io.Writer] interface. The input is expected to be log lines separated by newlines.
// If the log line is already valid JSON, the logger simply adds the id field. If the log line is not valid JSON,
// then the log line is wrapped in a JSON.
func (j *JSONLogger) Write(p []byte) (int, error) {
	// p may contain any number of log lines, so split the input by newline.
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		_, err := j.writeLine([]byte(line))
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (j *JSONLogger) writeLine(line []byte) (int, error) {
	log := map[string]interface{}{}

	err := json.Unmarshal(line, &log)
	if err != nil {
		// If the log is not a valid JSON, log it as a field of a valid JSON.
		return j.logln(string(line), map[string]string{})
	}

	// If the log is already in JSON format, just add in the id field.
	log[startTimeNanosField] = j.id
	logString, err := json.Marshal(log)
	if err != nil {
		return 0, err
	}
	return fmt.Fprintln(j.w, string(logString))
}
