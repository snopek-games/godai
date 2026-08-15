package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func results(model, surface string, attempts ...*Attempt) *Results {
	return &Results{Model: model, Surface: surface, Attempts: attempts, Summary: Summarize(attempts)}
}

func taskRow(markdown, task string) string {
	for line := range strings.SplitSeq(markdown, "\n") {
		if strings.HasPrefix(line, "| "+task+" ") {
			return line
		}
	}
	return ""
}

func attempt(task string, passed bool) *Attempt {
	return &Attempt{TaskID: task, TaskRev: "rev1", Passed: passed, ChecksTotal: 1,
		ChecksPassed: map[bool]int{true: 1, false: 0}[passed],
		Metrics:      &RunMetrics{WallTimeMS: 1000}}
}

func TestCompareMergesFilesIntoOneCell(t *testing.T) {
	is := is.New(t)

	c := NewComparison([]*Results{
		results("haiku", SurfaceMCP, attempt("a", true), attempt("b", false)),
		results("haiku", SurfaceMCP, attempt("a", true), attempt("b", true)),
	}, nil, 0.05)

	is.Equal(len(c.Cells), 1)
	is.Equal(c.Cells[0].Summary.Trials, 4)
	is.Equal(c.Cells[0].PassRate(), 0.75)

	passes, trials := c.Cells[0].Task("b")
	is.Equal(passes, 1)
	is.Equal(trials, 2)
}

func TestCompareOrdersCellsByModelThenSurface(t *testing.T) {
	is := is.New(t)

	c := NewComparison([]*Results{
		results("sonnet", SurfaceNone, attempt("a", false)),
		results("haiku", SurfaceCLI, attempt("a", true)),
		results("haiku", SurfaceMCP, attempt("a", true)),
	}, nil, 0.05)

	var got []string
	for _, cell := range c.Cells {
		got = append(got, cell.Key.String())
	}
	is.Equal(got, []string{"haiku/mcp", "haiku/cli", "sonnet/none"})
}

func TestCompareNotesUnevenTaskCoverage(t *testing.T) {
	is := is.New(t)

	c := NewComparison([]*Results{
		results("haiku", SurfaceMCP, attempt("a", true), attempt("b", true)),
		results("haiku", SurfaceCLI, attempt("a", true)),
	}, nil, 0.05)

	is.Equal(len(c.Notes), 1)
	is.True(strings.Contains(c.Notes[0], "haiku/cli"))
	is.True(strings.Contains(c.Notes[0], "b"))

	is.True(strings.Contains(taskRow(c.Markdown(), "b"), "—"))
}

func TestCompareNotesAnEditedTask(t *testing.T) {
	is := is.New(t)

	edited := attempt("a", true)
	edited.TaskRev = "rev2"

	c := NewComparison(
		[]*Results{results("haiku", SurfaceMCP, edited)},
		[]*Results{results("haiku", SurfaceMCP, attempt("a", false))},
		0.05)

	is.Equal(len(c.Notes), 1)
	is.True(strings.Contains(c.Notes[0], "changed between these runs"))
}

func TestCompareCallsOutOnlyRealRegressions(t *testing.T) {
	is := is.New(t)

	var passing, failing []*Attempt
	for range 40 {
		passing = append(passing, attempt("a", true))
		failing = append(failing, attempt("a", false))
	}

	// One attempt either way is noise; forty is not.
	noise := NewComparison(
		[]*Results{results("haiku", SurfaceMCP, attempt("a", false))},
		[]*Results{results("haiku", SurfaceMCP, attempt("a", true))},
		0.05)
	is.Equal(len(noise.Regressions()), 0)

	real := NewComparison(
		[]*Results{results("haiku", SurfaceMCP, failing...)},
		[]*Results{results("haiku", SurfaceMCP, passing...)},
		0.05)
	is.Equal(real.Regressions(), []CellKey{{"haiku", SurfaceMCP}})

	d, ok := real.Delta(real.Cells[0])
	is.True(ok)
	is.Equal(d.Points, -100.0)
	is.True(strings.Contains(real.Markdown(), "▼ -100"))
}

func TestCompareNamesEveryFailingRepeat(t *testing.T) {
	is := is.New(t)

	broke := attempt("a", false)
	broke.Repeat = 3
	broke.FailedChecks = []FailedCheck{{Name: "main_scene_loads", Detail: "main.tscn is missing or failed to parse"}}

	harness := attempt("b", false)
	harness.Repeat = 1
	harness.Error = "verify: verifier produced no report"

	md := NewComparison([]*Results{
		results("haiku", SurfaceNone, attempt("a", true), broke, harness),
	}, nil, 0.05).Markdown()

	is.True(strings.Contains(md, "### Failures and errors"))
	is.True(strings.Contains(md, "main_scene_loads: main.tscn is missing or failed to parse"))
	is.True(strings.Contains(md, "| r3 "))
	is.True(strings.Contains(md, "verifier produced no report"))
	is.True(!strings.Contains(md, "| r2 "))
}

func TestCompareCapsTheFailureListAndSaysSo(t *testing.T) {
	is := is.New(t)

	var attempts []*Attempt
	for range maxFailureRows + 5 {
		attempts = append(attempts, attempt("a", false))
	}

	md := NewComparison([]*Results{results("haiku", SurfaceNone, attempts...)}, nil, 0.05).Markdown()

	_, failures, _ := strings.Cut(md, "### Failures and errors")
	is.Equal(strings.Count(failures, "| haiku/none "), maxFailureRows)
	is.True(strings.Contains(md, "5 further failures are not listed"))
}

func TestLoadResultsReadsADirectory(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	blob, err := json.Marshal(results("haiku", SurfaceMCP, attempt("a", true)))
	is.NoErr(err)
	is.NoErr(os.WriteFile(filepath.Join(dir, "results-haiku.json"), blob, 0o644))
	is.NoErr(os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644))

	loaded, err := LoadResults([]string{dir})
	is.NoErr(err)
	is.Equal(len(loaded), 1)
	is.Equal(loaded[0].Model, "haiku")

	is.NoErr(os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{"hello":"world"}`), 0o644))
	_, err = LoadResults([]string{dir})
	is.True(err != nil)
}
