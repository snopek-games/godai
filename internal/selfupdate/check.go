package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const DefaultCheckInterval = 24 * time.Hour

type checkCache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
	// Notice throttling state, so every command doesn't nag (see notice.go).
	NotifiedVersion string    `json:"notified_version,omitempty"`
	NotifiedAt      time.Time `json:"notified_at,omitzero"`
	WarnedAt        time.Time `json:"warned_at,omitzero"`
}

// CheckCached reports the newest installable release when it's newer than the
// running version, asking GitLab at most once per interval and remembering the
// answer in cachePath in between.
func (u *Updater) CheckCached(ctx context.Context, cachePath string, interval time.Duration) (Version, bool, error) {
	if latest, ok := u.readCheckCache(cachePath, interval); ok {
		return latest, latest.Compare(u.currentVersion) > 0, nil
	}

	release, err := u.DetectLatest(ctx)
	if err != nil {
		// A release that nothing can be installed from isn't an error worth
		// bothering anyone about, but it shouldn't be cached either.
		if errors.Is(err, ErrNoRelease) {
			return Version{}, false, nil
		}
		return Version{}, false, err
	}

	u.writeCheckCache(cachePath, release.Version)

	return release.Version, u.IsNewer(release), nil
}

func loadCheckCache(cachePath string) checkCache {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Debug("unable to read the update check cache", "path", cachePath, "error", err)
		}
		return checkCache{}
	}

	var cache checkCache
	if err := json.Unmarshal(data, &cache); err != nil {
		slog.Debug("unable to parse the update check cache", "path", cachePath, "error", err)
		return checkCache{}
	}

	return cache
}

func (u *Updater) readCheckCache(cachePath string, interval time.Duration) (Version, bool) {
	cache := loadCheckCache(cachePath)
	if cache.LatestVersion == "" {
		return Version{}, false
	}

	age := time.Since(cache.CheckedAt)
	if age < 0 || age > interval {
		return Version{}, false
	}

	latest, err := ParseVersion(cache.LatestVersion)
	if err != nil {
		return Version{}, false
	}

	slog.Debug("using the cached update check", "latest", latest, "age", age)
	return latest, true
}

func (u *Updater) writeCheckCache(cachePath string, latest Version) {
	cache := loadCheckCache(cachePath)
	cache.CheckedAt = time.Now()
	cache.LatestVersion = latest.String()
	storeCheckCache(cachePath, cache)
}

func storeCheckCache(cachePath string, cache checkCache) {
	if err := saveCheckCache(cachePath, cache); err != nil {
		slog.Debug("unable to write the update check cache", "path", cachePath, "error", err)
	}
}

func saveCheckCache(cachePath string, cache checkCache) error {
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(cachePath), ".update-check-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	if err := os.Rename(temp.Name(), cachePath); err != nil {
		return fmt.Errorf("writing %s: %w", cachePath, err)
	}

	return nil
}
