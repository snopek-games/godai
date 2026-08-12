package godot

import (
	"testing"

	"github.com/matryer/is"
)

func TestParseEngineVersion(t *testing.T) {
	for _, tc := range []struct {
		input    string
		string   string
		tag      string
		template string
	}{
		{"4.5", "4.5-stable", "4.5-stable", "4.5.stable"},
		{"4.4.1", "4.4.1-stable", "4.4.1-stable", "4.4.1.stable"},
		{"4.5.0", "4.5-stable", "4.5-stable", "4.5.stable"},
		{"v4.5-stable", "4.5-stable", "4.5-stable", "4.5.stable"},
		{"4.6-beta3", "4.6-beta3", "4.6-beta3", "4.6.beta3"},
		{"4.8-dev3", "4.8-dev3", "4.8-dev3", "4.8.dev3"},
		{"4.8-dev", "4.8-dev", "4.8-dev", "4.8.dev"},
		{"4.7.2-rc1", "4.7.2-rc1", "4.7.2-rc1", "4.7.2.rc1"},
		{"4.5-mono", "4.5-stable-mono", "4.5-stable", "4.5.stable.mono"},
		{"4.5-stable-mono", "4.5-stable-mono", "4.5-stable", "4.5.stable.mono"},
		{"4.6-beta3-mono", "4.6-beta3-mono", "4.6-beta3", "4.6.beta3.mono"},
		{" 4.5-STABLE ", "4.5-stable", "4.5-stable", "4.5.stable"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			is := is.New(t)

			version, err := ParseEngineVersion(tc.input)
			is.NoErr(err)
			is.Equal(version.String(), tc.string)
			is.Equal(version.Tag(), tc.tag)
			is.Equal(version.TemplateDirName(), tc.template)
		})
	}
}

func TestParseVersionOutput(t *testing.T) {
	for _, tc := range []struct {
		input  string
		string string
	}{
		{"4.5.stable.official.a2b3c4d5e\n", "4.5-stable"},
		{"4.4.1.stable.mono.official.b09f793f5\n", "4.4.1-stable-mono"},
		{"4.6.beta3.official.f0ed4d0d1\n", "4.6-beta3"},
		{"3.5.3.stable.official.6c814135b\n", "3.5.3-stable"},
		{"4.5.stable.custom_build\n", "4.5-stable"},
		{"4.8.dev.custom_build.c03267c0a\n", "4.8-dev"},
		{"4.8.dev.mono.custom_build.c03267c0a\n", "4.8-dev-mono"},
		{"Warning: something happened\n4.5.stable.official.a2b3c4d5e\n", "4.5-stable"},
	} {
		t.Run(tc.string, func(t *testing.T) {
			is := is.New(t)

			version, err := ParseVersionOutput(tc.input)
			is.NoErr(err)
			is.Equal(version.String(), tc.string)
		})
	}
}

func TestParseVersionOutputRejects(t *testing.T) {
	for _, input := range []string{
		"",
		"4.5\n",
		"godot: command not found\n",
		"4.5.nightly1.official.a2b3c4d5e\n",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseVersionOutput(input); err == nil {
				t.Errorf("%q was accepted as version output", input)
			}
		})
	}
}

func TestParseEngineVersionRejects(t *testing.T) {
	for _, input := range []string{
		"",
		"4",
		"4.5.1.2",
		"four.five",
		"4.5-",
		"4.5-nightly1",
		"4.5-stable2",
		"my-build",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseEngineVersion(input); err == nil {
				t.Errorf("%q was accepted as a version", input)
			}
		})
	}
}

func TestEngineVersionCompare(t *testing.T) {
	is := is.New(t)

	sorted := []string{
		"4.5-dev1",
		"4.5-alpha1",
		"4.5-beta2",
		"4.5-beta10",
		"4.5-rc1",
		"4.5",
		"4.5-mono",
		"4.5.1",
		"4.6",
		"5.0",
	}

	for i := 1; i < len(sorted); i++ {
		before, err := ParseEngineVersion(sorted[i-1])
		is.NoErr(err)
		after, err := ParseEngineVersion(sorted[i])
		is.NoErr(err)

		if before.Compare(after) >= 0 {
			t.Errorf("%s should sort before %s", before, after)
		}
		if after.Compare(before) <= 0 {
			t.Errorf("%s should sort after %s", after, before)
		}
	}

	same, err := ParseEngineVersion("4.5-stable")
	is.NoErr(err)
	is.Equal(same.Compare(same), 0)
}
