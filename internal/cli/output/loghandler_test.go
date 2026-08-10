package output

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func logWith(level slog.Level, emit func(*slog.Logger)) string {
	buf := &bytes.Buffer{}
	emit(slog.New(NewLogHandler(buf, level, false)))
	return buf.String()
}

func TestHandlerWritesPlainLines(t *testing.T) {
	is := is.New(t)

	out := logWith(slog.LevelInfo, func(l *slog.Logger) {
		l.Warn("editor disconnected", "project", "/home/me/game")
	})

	is.Equal(out, "warning: editor disconnected (project=/home/me/game)\n")
	is.True(!strings.Contains(out, "time="))
	is.True(!strings.Contains(out, "level="))
}

func TestHandlerLabelsOnlyWarningsAndErrors(t *testing.T) {
	is := is.New(t)

	is.Equal(logWith(slog.LevelInfo, func(l *slog.Logger) { l.Info("connected to editor") }),
		"connected to editor\n")
	is.Equal(logWith(slog.LevelInfo, func(l *slog.Logger) { l.Error("nope") }),
		"error: nope\n")
}

func TestHandlerShowsAttributesAtEveryLevel(t *testing.T) {
	is := is.New(t)

	warn := logWith(slog.LevelWarn, func(l *slog.Logger) { l.Warn("hmm", "project", "/p") })
	is.True(strings.Contains(warn, "(project=/p)"))

	debug := logWith(slog.LevelDebug, func(l *slog.Logger) { l.Debug("hmm", "project", "/p") })
	is.True(strings.Contains(debug, "(project=/p)"))
}

func TestHandlerRespectsLevel(t *testing.T) {
	is := is.New(t)

	is.Equal(logWith(slog.LevelWarn, func(l *slog.Logger) { l.Info("connected to editor") }), "")
	is.True(logWith(slog.LevelWarn, func(l *slog.Logger) { l.Warn("careful") }) != "")
}

func TestHandlerWithAttrsAndGroup(t *testing.T) {
	is := is.New(t)

	buf := &bytes.Buffer{}
	logger := slog.New(NewLogHandler(buf, slog.LevelDebug, false)).
		WithGroup("editor").
		With("project", "/p")
	logger.Debug("hmm", "port", 9000)

	out := buf.String()
	is.True(strings.Contains(out, "editor.project=/p"))
	is.True(strings.Contains(out, "editor.port=9000"))
}
