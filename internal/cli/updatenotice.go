package cli

import (
	"context"
	"io"
	"os"
	"time"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/selfupdate"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

// noticeGrace is how long a finished command waits for a still-running update
// check. Long commands find the result already there; fast commands give up
// and let the cached result surface on a later run.
const noticeGrace = 500 * time.Millisecond

// Commands where an update notice would be unwelcome: mcp owns its stdio (and
// notifies through the editor instead), and self-update already talks about
// versions.
var noticeSkipCommands = map[string]bool{
	"":            true,
	"mcp":         true,
	"self-update": true,
	"help":        true,
	"h":           true,
	"completion":  true,
}

var activeNoticeCheck *selfupdate.NoticeCheck

func startUpdateNotice(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	if shouldCheckForUpdates(cmd) {
		if cachePath, err := core.GetUpdateCheckCachePath(); err == nil {
			activeNoticeCheck = selfupdate.StartNoticeCheck(core.Version, cachePath)
		}
	}
	return ctx, nil
}

// PrintUpdateNotice reports a newer release, when the background check found
// one. It runs from main() after everything else, so the notice is the last
// thing on the terminal.
func PrintUpdateNotice(w io.Writer) {
	if activeNoticeCheck == nil {
		return
	}
	activeNoticeCheck.PrintNotice(w, noticeGrace)
	activeNoticeCheck = nil
}

func shouldCheckForUpdates(cmd *cli.Command) bool {
	if noticeSkipCommands[cmd.Args().First()] {
		return false
	}
	if os.Getenv("GODAI_NO_UPDATE_CHECK") != "" || os.Getenv("CI") != "" {
		return false
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return false
	}
	if configPath, err := core.GetConfigPath(); err == nil && core.SavedUpdateCheckOff(configPath) {
		return false
	}
	return true
}
