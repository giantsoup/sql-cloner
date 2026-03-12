package core

import (
	"io"

	"github.com/charmbracelet/log"
)

func NewLogger(w io.Writer, debug bool) log.Logger {
	options := []log.LoggerOption{
		log.WithOutput(w),
	}
	if debug {
		options = append(options, log.WithLevel(log.DebugLevel))
	} else {
		options = append(options, log.WithLevel(log.InfoLevel))
	}
	return log.New(options...)
}
