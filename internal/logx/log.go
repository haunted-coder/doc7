package logx

import (
	"fmt"
	"io"
)

type Logger struct {
	Err     io.Writer
	Quiet   bool
	Verbose bool
}

func New(err io.Writer, quiet bool, verbose bool) Logger {
	return Logger{Err: err, Quiet: quiet, Verbose: verbose}
}

func (l Logger) Infof(format string, args ...interface{}) {
	if l.Quiet {
		return
	}
	fmt.Fprintf(l.Err, format+"\n", args...)
}

func (l Logger) Debugf(format string, args ...interface{}) {
	if l.Quiet || !l.Verbose {
		return
	}
	fmt.Fprintf(l.Err, format+"\n", args...)
}

func (l Logger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(l.Err, format+"\n", args...)
}
