package core

import (
	"context"
	"log/slog"
	"time"

	"gitlab.com/snopek-games/godai/internal/godot"
	"gitlab.com/snopek-games/godai/internal/selfupdate"
)

const updateAvailableNotification = "notifications/godai/update_available"

const updateCheckTimeout = 30 * time.Second

type updateAvailableParams struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
}

// CheckForUpdate looks for a newer release in the background and tells the
// connected editors about it, so the Godai panel can mention it. Editors that
// connect later are told by onEditorConnect().
func (s *Session) CheckForUpdate(ctx context.Context) {
	if s.config.UpdateCheckInterval <= 0 {
		return
	}

	updater, err := selfupdate.New(selfupdate.Config{CurrentVersion: Version})
	if err != nil {
		slog.Debug("not checking for updates", "error", err)
		return
	}

	cachePath, err := GetUpdateCheckCachePath()
	if err != nil {
		slog.Debug("not checking for updates", "error", err)
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	latest, newer, err := updater.CheckCached(checkCtx, cachePath, s.config.UpdateCheckInterval)
	cancel()
	if err != nil {
		slog.Debug("unable to check for updates", "error", err)
		return
	}
	if !newer {
		return
	}

	slog.Info("a newer version of godai is available",
		"latest", latest, "current", Version, "install", "godai self-update")

	s.updateMutex.Lock()
	s.updateAvailable = latest.String()
	s.updateMutex.Unlock()

	s.editorsMutex.RLock()
	connections := make([]*godot.Connection, 0, len(s.editors))
	for _, editor := range s.editors {
		connections = append(connections, editor.conn)
	}
	s.editorsMutex.RUnlock()

	for _, conn := range connections {
		s.sendUpdateNotification(ctx, conn)
	}
}

func (s *Session) sendUpdateNotification(ctx context.Context, conn *godot.Connection) {
	s.updateMutex.RLock()
	latest := s.updateAvailable
	s.updateMutex.RUnlock()

	if latest == "" {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := conn.SendNotification(ctx, updateAvailableNotification, updateAvailableParams{
		CurrentVersion: Version,
		LatestVersion:  latest,
	})
	if err != nil {
		slog.Debug("unable to tell the editor that an update is available", "error", err)
	}
}
