package core

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestDefaultToolsetsExcludesEngine(t *testing.T) {
	is := is.New(t)

	defaults := DefaultToolsets()
	is.Equal(len(defaults), len(ToolsetNames)-1) // everything but engine
	for _, name := range defaults {
		is.True(name != "engine")
	}
}

func TestExpandToolsets(t *testing.T) {
	is := is.New(t)

	enabled, err := ExpandToolsets(nil)
	is.NoErr(err)
	is.True(enabled["scene"]) // nil expands to the defaults
	is.True(enabled["project"])
	is.True(!enabled["engine"]) // engine is opt-in

	enabled, err = ExpandToolsets([]string{"default", "engine"})
	is.NoErr(err)
	is.True(enabled["scene"])
	is.True(enabled["engine"])

	enabled, err = ExpandToolsets([]string{"all"})
	is.NoErr(err)
	for _, name := range ToolsetNames {
		is.True(enabled[name])
	}

	enabled, err = ExpandToolsets([]string{"scene"})
	is.NoErr(err)
	is.True(enabled["scene"])
	is.True(!enabled["project"]) // one toolset doesn't drag in the defaults
}

func TestExpandToolsetsUnknownName(t *testing.T) {
	is := is.New(t)

	_, err := ExpandToolsets([]string{"bogus"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), `"bogus"`))
	is.True(strings.Contains(err.Error(), "default")) // the error suggests the valid names
	for _, name := range ToolsetNames {
		is.True(strings.Contains(err.Error(), name))
	}
}
