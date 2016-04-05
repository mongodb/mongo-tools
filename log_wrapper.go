package mongotape

import (
	"log"
	"os"
)

const (
	Always = iota
	Info
	DebugLow
	DebugHigh
)

var logger *log.Logger
var userInfoLogger *logWrapper
var toolDebugLogger *logWrapper

type logWrapper struct {
	out       *log.Logger
	verbosity int
}

func init() {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.Ldate|log.Ltime)
	}
	if userInfoLogger == nil {
		userInfoLogger = &logWrapper{logger, 0}
	}
	if toolDebugLogger == nil {
		toolDebugLogger = &logWrapper{logger, 0}
	}
}
func (lw *logWrapper) setVerbosity(verbose []bool) {
	lw.verbosity = len(verbose)
}

func (lw *logWrapper) Logf(minVerb int, format string, a ...interface{}) {
	if minVerb < 0 {
		panic("cannot set a minimum log verbosity that is less than 0")
	}

	if minVerb <= lw.verbosity {
		lw.out.Printf(format, a...)
	}
}

func (lw *logWrapper) Log(minVerb int, msg string) {
	if minVerb < 0 {
		panic("cannot set a minimum log verbosity that is less than 0")
	}

	if minVerb <= lw.verbosity {
		lw.out.Print(msg)
	}
}
func (lw *logWrapper) isInVerbosity(minVerb int) bool {
	return minVerb <= lw.verbosity
}
