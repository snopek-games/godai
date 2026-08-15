package eval

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// LoadResults reads result files, expanding a directory to the JSON files in it.
func LoadResults(paths []string) ([]*Results, error) {
	var files []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}
		matches, err := filepath.Glob(filepath.Join(path, "*.json"))
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no result files in %s", path)
		}
		files = append(files, matches...)
	}

	var out []*Results
	for _, file := range files {
		blob, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var r Results
		if err := json.Unmarshal(blob, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if r.Surface == "" {
			return nil, fmt.Errorf("%s: not a godai-eval results file", file)
		}
		r.File = file
		out = append(out, &r)
	}
	return out, nil
}

type CellKey struct{ Model, Surface string }

func (k CellKey) String() string { return k.Model + "/" + k.Surface }

// Cell is every attempt for one model on one surface, however many files they
// arrived in.
type Cell struct {
	Key      CellKey
	Files    []string
	Attempts []*Attempt
	Summary  Summary
}

func (c *Cell) PassRate() float64 { return rate(c.Summary.Passes, c.Summary.Trials) }

func (c *Cell) Task(id string) (passes, trials int) {
	for _, a := range c.Attempts {
		if a.TaskID == id {
			trials++
			if a.Passed {
				passes++
			}
		}
	}
	return passes, trials
}

func cells(results []*Results) []*Cell {
	byKey := map[CellKey]*Cell{}
	for _, r := range results {
		key := CellKey{r.Model, r.Surface}
		c := byKey[key]
		if c == nil {
			c = &Cell{Key: key}
			byKey[key] = c
		}
		c.Files = append(c.Files, r.File)
		c.Attempts = append(c.Attempts, r.Attempts...)
	}

	out := make([]*Cell, 0, len(byKey))
	for _, c := range byKey {
		// Recomputed rather than read from the files, each of which summarises
		// only itself.
		c.Summary = Summarize(c.Attempts)
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b *Cell) int {
		return cmp.Or(
			cmp.Compare(a.Key.Model, b.Key.Model),
			slices.Index(Surfaces, a.Key.Surface)-slices.Index(Surfaces, b.Key.Surface),
			cmp.Compare(a.Key.Surface, b.Key.Surface),
		)
	})
	return out
}

type Comparison struct {
	Cells    []*Cell
	Baseline map[CellKey]*Cell
	Alpha    float64

	Tasks []string

	// Notes are the reasons a column may not mean what it looks like.
	Notes []string
}

func NewComparison(results, baseline []*Results, alpha float64) *Comparison {
	c := &Comparison{Cells: cells(results), Alpha: alpha, Baseline: map[CellKey]*Cell{}}
	for _, cell := range cells(baseline) {
		c.Baseline[cell.Key] = cell
	}

	c.Tasks = taskIDs(c.Cells)
	c.Notes = append(c.Notes, coverageNotes(c.Cells, c.Tasks)...)
	c.Notes = append(c.Notes, revisionNotes(append(slices.Clone(c.Cells), cells(baseline)...))...)
	return c
}

func taskIDs(cells []*Cell) []string {
	seen := map[string]bool{}
	var ids []string
	for _, c := range cells {
		for _, a := range c.Attempts {
			if !seen[a.TaskID] {
				seen[a.TaskID] = true
				ids = append(ids, a.TaskID)
			}
		}
	}
	slices.Sort(ids)
	return ids
}

// Comparing a cell that ran five tasks against one that ran a subset is the
// easiest way to read a difference that isn't there.
func coverageNotes(cells []*Cell, tasks []string) []string {
	var notes []string
	for _, c := range cells {
		var missing []string
		for _, id := range tasks {
			if _, trials := c.Task(id); trials == 0 {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			notes = append(notes, fmt.Sprintf("%s never ran %s, so its rates are over a different set of tasks",
				c.Key, strings.Join(missing, ", ")))
		}
	}
	return notes
}

// A task edited between two runs makes their numbers incomparable, and looks
// exactly like the change under test having done something.
func revisionNotes(cells []*Cell) []string {
	revs := map[string]map[string]bool{}
	for _, c := range cells {
		for _, a := range c.Attempts {
			if revs[a.TaskID] == nil {
				revs[a.TaskID] = map[string]bool{}
			}
			revs[a.TaskID][a.TaskRev] = true
		}
	}

	var changed []string
	for id, seen := range revs {
		if len(seen) > 1 {
			changed = append(changed, id)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	slices.Sort(changed)
	return []string{fmt.Sprintf("%s changed between these runs, so the results either side of the edit are not comparable",
		strings.Join(changed, ", "))}
}

func (c *Comparison) Models() []string {
	var models []string
	for _, cell := range c.Cells {
		if !slices.Contains(models, cell.Key.Model) {
			models = append(models, cell.Key.Model)
		}
	}
	return models
}

// SurfaceColumns is the surfaces that were run, in the order they're worth
// reading: godai's own first, the baseline last.
func (c *Comparison) SurfaceColumns() []string {
	var surfaces []string
	for _, cell := range c.Cells {
		if !slices.Contains(surfaces, cell.Key.Surface) {
			surfaces = append(surfaces, cell.Key.Surface)
		}
	}
	return surfaces
}

func (c *Comparison) Cell(model, surface string) *Cell {
	for _, cell := range c.Cells {
		if cell.Key.Model == model && cell.Key.Surface == surface {
			return cell
		}
	}
	return nil
}

// Delta is one cell measured against the same cell in the baseline.
type Delta struct {
	Points float64 // percentage points of pass rate
	P      float64
}

func (c *Comparison) Delta(cell *Cell) (Delta, bool) {
	base, ok := c.Baseline[cell.Key]
	if !ok {
		return Delta{}, false
	}
	return Delta{
		Points: (cell.PassRate() - base.PassRate()) * 100,
		P:      twoProportionP(base.PassRate(), base.Summary.Trials, cell.PassRate(), cell.Summary.Trials),
	}, true
}

// Regressions names the cells that dropped by more than the run-to-run noise.
func (c *Comparison) Regressions() []CellKey {
	var keys []CellKey
	for _, cell := range c.Cells {
		if d, ok := c.Delta(cell); ok && d.Points < 0 && d.P < c.Alpha {
			keys = append(keys, cell.Key)
		}
	}
	return keys
}

// twoProportionP is a two-sided z-test on the difference of two proportions:
// with a handful of tasks and a few repeats, a swing of several points is
// routinely noise, and gating on raw deltas means chasing ghosts.
func twoProportionP(p1 float64, n1 int, p2 float64, n2 int) float64 {
	if n1 == 0 || n2 == 0 {
		return 1
	}
	f1, f2 := float64(n1), float64(n2)
	pooled := (p1*f1 + p2*f2) / (f1 + f2)
	se := math.Sqrt(pooled * (1 - pooled) * (1/f1 + 1/f2))
	if se == 0 {
		return 1
	}
	z := (p2 - p1) / se
	return 2 * (1 - 0.5*(1+math.Erf(math.Abs(z)/math.Sqrt2)))
}

func rate(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}
