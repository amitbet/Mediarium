package logbuf

import (
	"log"
	"log/slog"
	"strings"
)

// Level is the lowest level the app writes to its log. It starts at info;
// SetDebug moves it to debug and back while the app runs.
var Level = new(slog.LevelVar)

// SetDebug switches detailed (debug) logging on or off at once.
func SetDebug(on bool) {
	if on {
		Level.Set(slog.LevelDebug)
	} else {
		Level.Set(slog.LevelInfo)
	}
}

// Debug reports whether detailed logging is on.
func Debug() bool { return Level.Level() <= slog.LevelDebug }

// logWriter hands each line the slog handler writes to the standard log
// package, so it gets the same time stamp and the same destination (stderr
// and the in-memory copy, both with secrets taken out) as every other line.
type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	_ = log.Output(2, strings.TrimRight(string(p), "\r\n"))
	return len(p), nil
}

// InstallSlog makes slog.Debug, Info, Warn and Error follow Level and write
// through the standard log package. Call it after log.SetOutput. slog.SetDefault
// would send the log package's own lines through the handler too, so the log
// package's settings are put back afterwards.
func InstallSlog() {
	flags, out := log.Flags(), log.Writer()
	h := slog.NewTextHandler(logWriter{}, &slog.HandlerOptions{
		Level: Level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	slog.SetDefault(slog.New(h))
	log.SetFlags(flags)
	log.SetOutput(out)
}
