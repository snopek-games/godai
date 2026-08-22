package godot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

// Godot escapes only ']' in a section name, which is what lets a Windows path
// be one. The wanted bytes are what Godot 4.6.3 writes for the same name.
func TestConfigFileSectionNameEscaping(t *testing.T) {
	cases := []struct {
		name    string
		section string
		want    string
	}{
		{"Plain", "section", "[section]"},
		{"WindowsPath", `C:\Users\me\proj`, `[C:\Users\me\proj]`},
		{"Bracket", "has]bracket", `[has\]bracket]`},
		{"BackslashThenBracket", `esc\]seq`, `[esc\\]seq]`},
		{"BackslashPair", `double\\back`, `[double\\back]`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			is := is.New(t)

			c := NewConfigFile()
			c.Set(tc.section, "favorite", false)

			s, err := c.String()
			is.NoErr(err)
			is.Equal(strings.Split(s, "\n")[0], tc.want)

			back, err := ParseConfigFile(s)
			is.NoErr(err)
			is.Equal(back.ListSections(), []string{tc.section})
		})
	}
}

func TestConfigFileRoundtrip(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		t.Run(e.Name(), func(t *testing.T) {
			t.Parallel()
			is := is.New(t)

			path := filepath.Join("testdata", e.Name())
			data, err := os.ReadFile(path)
			is.NoErr(err)

			sdata := string(data)

			c, err := ParseConfigFile(sdata)
			is.NoErr(err)

			s, err := c.String()
			is.NoErr(err)

			want := strings.Split(strings.TrimSpace(sdata), "\n")
			got := strings.Split(strings.TrimSpace(s), "\n")

			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
