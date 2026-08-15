package core

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestDefaultToolsetsExcludesEngine(t *testing.T) {
	is := is.New(t)

	defaults := DefaultToolsets()
	is.Equal(len(defaults), len(ToolsetNames)-1)
	for _, name := range defaults {
		is.True(name != "engine")
	}
}

func TestExpandToolsets(t *testing.T) {
	is := is.New(t)

	enabled, err := ExpandToolsets(nil)
	is.NoErr(err)
	is.True(enabled["scene"])
	is.True(enabled["project"])
	is.True(!enabled["engine"])

	enabled, err = ExpandToolsets([]string{"default", "engine"})
	is.NoErr(err)
	is.True(enabled["scene"])
	is.True(enabled["engine"])

	enabled, err = ExpandToolsets([]string{"scene"})
	is.NoErr(err)
	is.True(enabled["scene"])
	is.True(!enabled["project"])
}

func TestExpandToolsetsUnknownName(t *testing.T) {
	is := is.New(t)

	_, err := ExpandToolsets([]string{"bogus"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), `"bogus"`))
	is.True(strings.Contains(err.Error(), "default"))
	for _, name := range ToolsetNames {
		is.True(strings.Contains(err.Error(), name))
	}
}
