package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func fakeGodai(t *testing.T, script string) *Workspace {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "godai")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Workspace{Root: root, Project: filepath.Join(root, "project"), cfg: Config{GodaiBin: bin}}
}

func TestVerifyEditorParsesTheReportOutOfTheScriptOutput(t *testing.T) {
	is := is.New(t)

	w := fakeGodai(t, `echo '{"structuredContent":{"success":true,"output":["noise","GODAI_VERIFY_JSON:{\"passed\":false,\"checks_passed\":1,\"checks_total\":2,\"checks\":[{\"name\":\"a\",\"ok\":true},{\"name\":\"b\",\"ok\":false,\"detail\":\"nope\"}]}"]}}'`)

	rep, err := VerifyEditor(context.Background(), w, &Spec{Dir: w.Root})
	is.NoErr(err)
	is.True(!rep.Passed)
	is.Equal(rep.ChecksPassed, 1)
	is.Equal(rep.ChecksTotal, 2)
	is.Equal(rep.Checks[1].Detail, "nope")
}

func TestVerifyEditorSurfacesStderrWhenGodaiFails(t *testing.T) {
	is := is.New(t)

	w := fakeGodai(t, `echo "no editor is running" >&2; exit 1`)

	_, err := VerifyEditor(context.Background(), w, &Spec{Dir: w.Root})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "no editor is running"))
}

func TestVerifyEditorFailsWhenTheScriptPrintsNoReport(t *testing.T) {
	is := is.New(t)

	w := fakeGodai(t, `echo '{"structuredContent":{"success":true,"output":["just some prints"]}}'`)

	_, err := VerifyEditor(context.Background(), w, &Spec{Dir: w.Root})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "no report"))
}

func TestMergeReportsCombinesChecksAndRequiresBothToPass(t *testing.T) {
	is := is.New(t)

	merged := MergeReports(
		&VerifyReport{Passed: true, ChecksPassed: 2, ChecksTotal: 2,
			Checks: []VerifyCheck{{Name: "a", OK: true}, {Name: "b", OK: true}}, Log: "first"},
		&VerifyReport{Passed: false, ChecksPassed: 0, ChecksTotal: 1,
			Checks: []VerifyCheck{{Name: "c", Detail: "nope"}}, Log: "second"},
	)

	is.True(!merged.Passed)
	is.Equal(merged.ChecksPassed, 2)
	is.Equal(merged.ChecksTotal, 3)
	is.Equal(len(merged.Checks), 3)
	is.Equal(merged.Log, "first\nsecond")
}
