package selfupdate

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
)

func noticeOutput(t *testing.T, updater *Updater, currentVersion, cachePath string) string {
	t.Helper()

	buf := &bytes.Buffer{}
	check := startNoticeCheck(updater, nil, currentVersion, cachePath)
	check.PrintNotice(buf, 5*time.Second, false)
	return buf.String()
}

func TestNoticePrintsOncePerDay(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater := newTestUpdater(t, fake, "0.3.0")
	cachePath := filepath.Join(t.TempDir(), "update-check.json")

	out := noticeOutput(t, updater, "0.3.0", cachePath)
	is.True(strings.Contains(out, "godai 0.4.0 is available"))
	is.True(strings.Contains(out, "self-update"))

	is.Equal(noticeOutput(t, updater, "0.3.0", cachePath), "") // no second notice the same day
}

func TestNoticeQuietWhenUpToDate(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.3.0"}})
	updater := newTestUpdater(t, fake, "0.3.0")
	cachePath := filepath.Join(t.TempDir(), "update-check.json")

	is.Equal(noticeOutput(t, updater, "0.3.0", cachePath), "")
}

func TestNoticeRepeatsForAnEvenNewerVersion(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.5.0"}})
	updater := newTestUpdater(t, fake, "0.3.0")
	cachePath := filepath.Join(t.TempDir(), "update-check.json")

	storeCheckCache(cachePath, checkCache{
		CheckedAt:       time.Now().Add(-25 * time.Hour),
		LatestVersion:   "0.4.0",
		NotifiedVersion: "0.4.0",
		NotifiedAt:      time.Now(),
	})

	out := noticeOutput(t, updater, "0.3.0", cachePath)
	is.True(strings.Contains(out, "godai 0.5.0 is available"))
}

func TestNoticeWarnsOncePerDayWhenTheCheckFails(t *testing.T) {
	is := is.New(t)

	updater, err := New(Config{
		CurrentVersion: "0.3.0",
		BaseURL:        "https://127.0.0.1:1/api/v4",
		Variant:        testVariant,
	})
	is.NoErr(err)
	cachePath := filepath.Join(t.TempDir(), "update-check.json")

	out := noticeOutput(t, updater, "0.3.0", cachePath)
	is.True(strings.Contains(out, "warning:"))
	is.True(strings.Contains(out, "unable to check for godai updates"))

	is.Equal(noticeOutput(t, updater, "0.3.0", cachePath), "") // no second warning the same day
}

func TestNoticeAbandonsASlowCheck(t *testing.T) {
	is := is.New(t)

	check := &NoticeCheck{outcome: make(chan noticeOutcome)}

	buf := &bytes.Buffer{}
	check.PrintNotice(buf, time.Millisecond, false)
	is.Equal(buf.String(), "") // gave up without printing anything
}
