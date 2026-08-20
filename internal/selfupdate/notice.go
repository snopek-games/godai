package selfupdate

import (
	"context"
	"fmt"
	"io"
	"time"

	"gitlab.com/snopek-games/godai/internal/cli/output"
)

const noticeCheckTimeout = 10 * time.Second

// NoticeInterval throttles the "new version available" notice and the
// check-failure warning, so at most one of each per day reaches the user.
const NoticeInterval = 24 * time.Hour

type noticeOutcome struct {
	latest Version
	newer  bool
	err    error
}

// A NoticeCheck looks for a newer release in the background, so a command can
// run while the check happens and print a notice on its way out.
type NoticeCheck struct {
	cachePath      string
	currentVersion string
	outcome        chan noticeOutcome
}

func StartNoticeCheck(currentVersion, cachePath string) *NoticeCheck {
	updater, err := New(Config{CurrentVersion: currentVersion})
	return startNoticeCheck(updater, err, currentVersion, cachePath)
}

func startNoticeCheck(updater *Updater, err error, currentVersion, cachePath string) *NoticeCheck {
	n := &NoticeCheck{
		cachePath:      cachePath,
		currentVersion: currentVersion,
		outcome:        make(chan noticeOutcome, 1),
	}

	go func() {
		if err != nil {
			n.outcome <- noticeOutcome{err: err}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), noticeCheckTimeout)
		defer cancel()

		latest, newer, err := updater.CheckCached(ctx, cachePath, DefaultCheckInterval)
		n.outcome <- noticeOutcome{latest: latest, newer: newer, err: err}
	}()

	return n
}

// A check that hasn't finished within grace is abandoned; its result still
// lands in the cache for a later command to report.
func (n *NoticeCheck) PrintNotice(w io.Writer, grace time.Duration, color bool) {
	var outcome noticeOutcome
	select {
	case outcome = <-n.outcome:
	case <-time.After(grace):
		return
	}

	cache := loadCheckCache(n.cachePath)
	now := time.Now()

	if outcome.err != nil {
		if now.Sub(cache.WarnedAt) < NoticeInterval {
			return
		}
		cache.WarnedAt = now
		storeCheckCache(n.cachePath, cache)
		fmt.Fprintf(w, "\n%s unable to check for godai updates: %v\n", output.Paint(color, output.Yellow, "warning:"), outcome.err)
		return
	}

	if !outcome.newer {
		return
	}

	latest := outcome.latest.String()
	if cache.NotifiedVersion == latest && now.Sub(cache.NotifiedAt) < NoticeInterval {
		return
	}
	cache.NotifiedVersion = latest
	cache.NotifiedAt = now
	storeCheckCache(n.cachePath, cache)

	instruction := "run 'godai self-update' to install it"
	if exePath, err := ExecutablePath(); err == nil {
		instruction = InstallInstruction(exePath)
	}
	fmt.Fprintf(w, "\ngodai %s is available (currently running %s); %s\n", output.Paint(color, output.Cyan, latest), output.Paint(color, output.Cyan, n.currentVersion), instruction)
}
