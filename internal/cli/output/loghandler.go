package output

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

type LogHandler struct {
	mutex  *sync.Mutex
	out    io.Writer
	level  slog.Level
	color  bool
	prefix string
	attrs  []slog.Attr
}

func NewLogHandler(out io.Writer, level slog.Level, color bool) *LogHandler {
	return &LogHandler{
		mutex: &sync.Mutex{},
		out:   out,
		level: level,
		color: color,
	}
}

func (h *LogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

func (h *LogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.prefix = h.prefix + name + "."
	return &next
}

func (h *LogHandler) Handle(_ context.Context, record slog.Record) error {
	line := &strings.Builder{}

	if label, color := levelLabel(record.Level); label != "" {
		if h.color {
			fmt.Fprintf(line, "\x1b[%sm%s:\x1b[0m ", color, label)
		} else {
			fmt.Fprintf(line, "%s: ", label)
		}
	}

	line.WriteString(record.Message)

	h.writeAttrs(line, record)

	line.WriteString("\n")

	h.mutex.Lock()
	defer h.mutex.Unlock()
	_, err := io.WriteString(h.out, line.String())
	return err
}

func (h *LogHandler) writeAttrs(line *strings.Builder, record slog.Record) {
	parts := make([]string, 0, len(h.attrs)+record.NumAttrs())
	for _, attr := range h.attrs {
		parts = append(parts, fmt.Sprintf("%s%s=%v", h.prefix, attr.Key, attr.Value))
	}
	record.Attrs(func(attr slog.Attr) bool {
		parts = append(parts, fmt.Sprintf("%s%s=%v", h.prefix, attr.Key, attr.Value))
		return true
	})

	if len(parts) > 0 {
		fmt.Fprintf(line, " (%s)", strings.Join(parts, " "))
	}
}

func levelLabel(level slog.Level) (label, color string) {
	switch {
	case level >= slog.LevelError:
		return "error", "31"
	case level >= slog.LevelWarn:
		return "warning", "33"
	default:
		return "", ""
	}
}
