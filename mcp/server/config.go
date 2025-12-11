package server

import "time"

type Config struct {
	EditorBasePort   int
	EditorPortCount  int
	EditorRetryDelay time.Duration
	EditorTimeout    time.Duration
	DefaultGodotPath string
	ProjectBasePath  string
	X11Display       string
	Debug            bool
}

// @todo We should be able to store config to a file (with some rich info than here)
//       For example, we should track projects recently opened, Godot executeables used, etc
