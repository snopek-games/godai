package selfupdate

import (
	"testing"

	"github.com/matryer/is"
)

func TestParseVersion(t *testing.T) {
	is := is.New(t)

	for _, test := range []struct {
		input      string
		want       Version
		wantString string
	}{
		{"1.2.3", Version{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
		{"0.0.0", Version{}, "0.0.0"},
		{"v0.4.0-beta1", Version{Minor: 4, Prerelease: "beta1"}, "0.4.0-beta1"},
		{"v0.4.0-rc.2", Version{Minor: 4, Prerelease: "rc.2"}, "0.4.0-rc.2"},
		{"1.2.3+build.5", Version{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
		{"1.2.3-beta.1+build.5", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta.1"}, "1.2.3-beta.1"},
		{"10.20.30", Version{Major: 10, Minor: 20, Patch: 30}, "10.20.30"},
	} {
		got, err := ParseVersion(test.input)
		is.NoErr(err)
		is.Equal(got, test.want)
		is.Equal(got.String(), test.wantString)
	}
}

func TestParseVersionRejectsGarbage(t *testing.T) {
	is := is.New(t)

	for _, input := range []string{
		"",
		"1.2",
		"1.2.3.4",
		"1.2.x",
		"one.two.three",
		"v",
		"1.2.3-",
		" 1.2.3",
		"1.2.-3",
	} {
		_, err := ParseVersion(input)
		is.True(err != nil)
	}
}

func TestVersionCompare(t *testing.T) {
	is := is.New(t)

	for _, test := range []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"0.3.2", "0.4.0", -1},
		{"0.4.0-beta1", "0.4.0", -1},
		{"0.4.0", "0.4.0-beta1", 1},
		{"0.4.0-beta1", "0.3.2", 1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
	} {
		a, err := ParseVersion(test.a)
		is.NoErr(err)
		b, err := ParseVersion(test.b)
		is.NoErr(err)

		is.Equal(a.Compare(b), test.want)
		is.Equal(b.Compare(a), -test.want)
	}
}

func TestVersionIsPrerelease(t *testing.T) {
	is := is.New(t)

	stable, err := ParseVersion("0.4.0")
	is.NoErr(err)
	is.True(!stable.IsPrerelease())

	beta, err := ParseVersion("0.4.0-beta1")
	is.NoErr(err)
	is.True(beta.IsPrerelease())
}
