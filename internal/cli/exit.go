package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

const (
	ExitOK            = 0
	ExitFailure       = 1
	ExitUsage         = 2
	ExitNotConfigured = 3
	ExitNoEditor      = 4
	ExitTimeout       = 5
	ExitToolFailed    = 6
	ExitInterrupted   = 130
)

type usageError struct{ error }

func newUsageError(format string, args ...any) error {
	return usageError{fmt.Errorf(format, args...)}
}

type propagatedExit struct{ code int }

func (propagatedExit) Error() string { return "" }

func ExitCodeFor(err error) int {
	var propagated propagatedExit
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, &propagated):
		return propagated.code
	case errors.Is(err, context.Canceled):
		return ExitInterrupted
	case errors.As(err, &usageError{}), isUnknownHelpTopic(err):
		return ExitUsage
	case errors.Is(err, core.ErrNotConfigured):
		return ExitNotConfigured
	case errors.Is(err, core.ErrNoEditor):
		return ExitNoEditor
	case errors.Is(err, core.ErrToolFailed):
		return ExitToolFailed
	case errors.Is(err, context.DeadlineExceeded):
		return ExitTimeout
	default:
		return ExitFailure
	}
}

// `godai help <typo>` is answered by urfave/cli itself, with an exit code that
// happens to be our ExitNotConfigured.
func isUnknownHelpTopic(err error) bool {
	var exitCoder cli.ExitCoder
	return errors.As(err, &exitCoder) && exitCoder.ExitCode() == 3
}

func PrintError(w io.Writer, err error, asJSON, verbose bool) {
	if errors.As(err, &propagatedExit{}) {
		return
	}

	var userErr *core.UserError
	hasUserErr := errors.As(err, &userErr)

	userMessage := ""
	if hasUserErr {
		userMessage = userErr.UserMessage()
		if verbose {
			userMessage = userErr.FullMessage()
		}
	}

	if asJSON {
		payload := struct {
			Error struct {
				Message   string   `json:"message"`
				Solutions []string `json:"solutions,omitempty"`
				Code      int      `json:"code"`
			} `json:"error"`
		}{}
		payload.Error.Message = err.Error()
		payload.Error.Code = ExitCodeFor(err)
		if hasUserErr {
			payload.Error.Message = userMessage
			payload.Error.Solutions = userErr.Solutions
		}

		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		encoder.Encode(payload)
		return
	}

	prefix := output.Paint(useColor(), output.Red, "godai:")

	if hasUserErr {
		fmt.Fprintf(w, "%s %s\n", prefix, userMessage)
		if len(userErr.Solutions) > 0 {
			fmt.Fprintln(w, "\nPossible solutions:")
			for _, s := range userErr.Solutions {
				fmt.Fprintf(w, "  - %s\n", s)
			}
		}
		return
	}

	fmt.Fprintf(w, "%s %v\n", prefix, err)
}
