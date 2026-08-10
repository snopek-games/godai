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

func (u *Updater) readCheckCache(cachePath string, interval time.Duration) (Version, bool) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Debug("unable to read the update check cache", "path", cachePath, "error", err)
		}
		return Version{}, false
	}

	var cache checkCache
	if err := json.Unmarshal(data, &cache); err != nil {
		slog.Debug("unable to parse the update check cache", "path", cachePath, "error", err)
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
	if err := u.saveCheckCache(cachePath, latest); err != nil {
		slog.Debug("unable to write the update check cache", "path", cachePath, "error", err)
	}
}

func (u *Updater) saveCheckCache(cachePath string, latest Version) error {
	data, err := json.Marshal(checkCache{
		CheckedAt:     time.Now(),
		LatestVersion: latest.String(),
	})
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
