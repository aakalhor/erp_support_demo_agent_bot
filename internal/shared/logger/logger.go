// Package logger is a thin wrapper around log.Logger so the rest of
// the codebase doesn't depend directly on the standard log package.
// Keeping it tiny is intentional: this is an MVP, not an observability
// platform.
package logger

import (
	"log"
	"os"
)

// Logger is the project-wide logger interface.
type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

type stdLogger struct {
	info *log.Logger
	err  *log.Logger
}

// New returns a logger that writes to stdout/stderr with a short prefix.
func New(prefix string) Logger {
	return &stdLogger{
		info: log.New(os.Stdout, "["+prefix+"][INFO] ", log.LstdFlags),
		err:  log.New(os.Stderr, "["+prefix+"][ERR ] ", log.LstdFlags),
	}
}

func (l *stdLogger) Infof(format string, args ...any)  { l.info.Printf(format, args...) }
func (l *stdLogger) Errorf(format string, args ...any) { l.err.Printf(format, args...) }
