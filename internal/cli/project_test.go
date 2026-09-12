package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

func TestWarnIgnoredOpenFlags(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		result core.OpenProjectResult
		want   string
	}{
		{"not already open", []string{"--headless", "--auto-approve"}, core.OpenProjectResult{}, ""},
		{"no flags", nil, core.OpenProjectResult{AlreadyOpen: true}, ""},
		{"auto approve", []string{"--auto-approve"}, core.OpenProjectResult{AlreadyOpen: true}, "--auto-approve had no effect"},
		{"headless", []string{"--headless"}, core.OpenProjectResult{AlreadyOpen: true}, "--headless had no effect"},
		{"headless already", []string{"--headless"}, core.OpenProjectResult{AlreadyOpen: true, Headless: true}, ""},
		{"both", []string{"--headless", "--auto-approve"}, core.OpenProjectResult{AlreadyOpen: true}, "--headless and --auto-approve had no effect"},
		{"offscreen", []string{"--offscreen"}, core.OpenProjectResult{AlreadyOpen: true}, "--offscreen had no effect"},
		{"offscreen already", []string{"--offscreen"}, core.OpenProjectResult{AlreadyOpen: true, Offscreen: true}, ""},
		{"offscreen size already", []string{"--offscreen", "--offscreen-size", "800x600"}, core.OpenProjectResult{AlreadyOpen: true, Offscreen: true}, "--offscreen-size had no effect"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			warning := openFlagWarning(t, tc.args, tc.result)
			switch {
			case tc.want == "" && warning != "":
				t.Errorf("unexpected warning: %s", warning)
			case tc.want != "" && !strings.Contains(warning, tc.want):
				t.Errorf("got %q, want it to mention %q", warning, tc.want)
			}
		})
	}
}

func openFlagWarning(t *testing.T, args []string, result core.OpenProjectResult) string {
	t.Helper()

	errOut := &bytes.Buffer{}
	open := projectCommand("").Command("open")
	open.Action = func(_ context.Context, cmd *cli.Command) error {
		warnIgnoredOpenFlags(output.NewPrinter(io.Discard, errOut, false), cmd, &result)
		return nil
	}

	if err := open.Run(context.Background(), append([]string{"open", "/nope"}, args...)); err != nil {
		t.Fatal(err)
	}
	return errOut.String()
}
