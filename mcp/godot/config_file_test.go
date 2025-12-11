package godot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

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
