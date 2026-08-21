package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/core"
)

func TestPrintErrorUserError(t *testing.T) {
	is := is.New(t)

	err := core.NewUserError("something failed", errors.New("boom"), []string{"try again"})

	out := &strings.Builder{}
	PrintError(out, err, false, false)
	is.Equal(out.String(), "godai: something failed\n\nPossible solutions:\n  - try again\n")

	out.Reset()
	PrintError(out, err, false, true)
	is.Equal(out.String(), "godai: something failed: boom\n\nPossible solutions:\n  - try again\n")
}

func TestPrintErrorUserErrorJSON(t *testing.T) {
	is := is.New(t)

	err := core.NewUserError("something failed", errors.New("boom"), []string{"try again"})

	payload := struct {
		Error struct {
			Message   string
			Solutions []string
			Code      int
		}
	}{}

	out := &strings.Builder{}
	PrintError(out, err, true, false)
	is.NoErr(json.Unmarshal([]byte(out.String()), &payload))
	is.Equal(payload.Error.Message, "something failed")
	is.Equal(payload.Error.Solutions, []string{"try again"})
	is.Equal(payload.Error.Code, ExitFailure)

	out.Reset()
	PrintError(out, err, true, true)
	is.NoErr(json.Unmarshal([]byte(out.String()), &payload))
	is.Equal(payload.Error.Message, "something failed: boom")
}

func TestPrintErrorPlainError(t *testing.T) {
	is := is.New(t)

	out := &strings.Builder{}
	PrintError(out, errors.New("boom"), false, false)
	is.Equal(out.String(), "godai: boom\n")

	out.Reset()
	PrintError(out, errors.New("boom"), false, true)
	is.Equal(out.String(), "godai: boom\n") // verbose changes nothing without a UserError
}
