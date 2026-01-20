package console

import (
	"fmt"
	"io"
)

type Logger struct {
	w io.Writer
}

func New(w io.Writer) Logger {
	return Logger{w: w}
}

func (l Logger) Info(message string) {
	l.write("info", message)
}

func (l Logger) Warn(message string) {
	l.write("warn", message)
}

func (l Logger) Error(message string) {
	l.write("error", message)
}

func (l Logger) write(level string, message string) {
	if l.w == nil {
		return
	}
	_, _ = fmt.Fprintf(l.w, "[%s] %s\n", level, message)
}
