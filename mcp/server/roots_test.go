package server

import (
	"runtime"
	"testing"
)

func TestFileURIToPath(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want string
		// onlyOn restricts the case to a single GOOS (for OS-specific path forms).
		onlyOn string
	}{
		{name: "absolute", uri: "file:///home/me/proj", want: "/home/me/proj", onlyOn: "linux"},
		{name: "percent-encoded space", uri: "file:///home/me/My%20Project", want: "/home/me/My Project", onlyOn: "linux"},
		{name: "non-file uri passes through", uri: "vscode://foo", want: "vscode://foo"},
		{name: "plain path passes through", uri: "/home/me/proj", want: "/home/me/proj"},
		{name: "windows drive", uri: "file:///C:/Users/me/proj", want: `C:\Users\me\proj`, onlyOn: "windows"},
		{name: "windows drive with space", uri: "file:///C:/Program%20Files/proj", want: `C:\Program Files\proj`, onlyOn: "windows"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.onlyOn != "" && runtime.GOOS != tc.onlyOn {
				t.Skipf("only relevant on %s", tc.onlyOn)
			}
			if got := fileURIToPath(tc.uri); got != tc.want {
				t.Errorf("fileURIToPath(%q) = %q, want %q", tc.uri, got, tc.want)
			}
		})
	}
}
