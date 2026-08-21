package selfupdate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matryer/is"
)

func newCheckUpdater(t *testing.T, fake *fakeGitLab, currentVersion string) (*Updater, string) {
	t.Helper()

	return newTestUpdater(t, fake, currentVersion), filepath.Join(t.TempDir(), "update-check.json")
}

func TestCheckCached(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}, {tag: "v0.3.2"}})
	updater, cachePath := newCheckUpdater(t, fake, "0.3.2")

	latest, newer, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(newer)
	is.Equal(latest.String(), "0.4.0")

	requests := fake.releaseRequests.Load()
	is.True(requests > 0) // the first check hit the network

	// The second check inside the interval comes from the cache.
	latest, newer, err = updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(newer)
	is.Equal(latest.String(), "0.4.0")
	is.Equal(fake.releaseRequests.Load(), requests)
}

func TestCheckCachedWhenUpToDate(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater, cachePath := newCheckUpdater(t, fake, "0.4.0")

	latest, newer, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(!newer)
	is.Equal(latest.String(), "0.4.0")
}

// The cache holds the latest release rather than a yes/no answer, so it still
// gives the right answer once the running version catches up with it.
func TestCheckCachedAfterUpdating(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater, cachePath := newCheckUpdater(t, fake, "0.3.2")

	_, newer, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(newer)

	updated := newTestUpdater(t, fake, "0.4.0")
	_, newer, err = updated.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(!newer)
	is.Equal(fake.releaseRequests.Load(), int64(1)) // the second check reused the cache
}

func TestCheckCachedExpires(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater, cachePath := newCheckUpdater(t, fake, "0.3.2")

	_, _, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.Equal(fake.releaseRequests.Load(), int64(1))

	writeCheckCacheAt(t, cachePath, "0.4.0", time.Now().Add(-2*time.Hour))

	_, newer, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(newer)
	is.Equal(fake.releaseRequests.Load(), int64(2)) // the expired cache forced a fresh check
}

func TestCheckCachedWithAnUnusableCache(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater, cachePath := newCheckUpdater(t, fake, "0.3.2")

	for _, contents := range []string{
		"not json",
		`{"checked_at": "2026-01-01T00:00:00Z", "latest_version": "nonsense"}`,
	} {
		is.NoErr(os.WriteFile(cachePath, []byte(contents), 0o644))

		_, newer, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
		is.NoErr(err)
		is.True(newer) // a bad cache falls back to a live check
	}
}

// A clock that jumped backwards shouldn't leave the cache valid forever.
func TestCheckCachedIgnoresFutureTimestamps(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater, cachePath := newCheckUpdater(t, fake, "0.3.2")

	writeCheckCacheAt(t, cachePath, "9.9.9", time.Now().Add(24*time.Hour))

	latest, newer, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(newer)
	is.Equal(latest.String(), "0.4.0") // refetched, not the future-stamped 9.9.9
}

func TestCheckCachedWithNoUsableRelease(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0", omitChecksums: true}})
	updater, cachePath := newCheckUpdater(t, fake, "0.3.2")

	_, newer, err := updater.CheckCached(context.Background(), cachePath, time.Hour)
	is.NoErr(err)
	is.True(!newer)

	_, err = os.Stat(cachePath)
	is.True(os.IsNotExist(err)) // nothing worth caching
}

func writeCheckCacheAt(t *testing.T, cachePath, latest string, checkedAt time.Time) {
	t.Helper()
	is := is.New(t)

	data, err := json.Marshal(checkCache{CheckedAt: checkedAt, LatestVersion: latest})
	is.NoErr(err)
	is.NoErr(os.WriteFile(cachePath, data, 0o644))
}
