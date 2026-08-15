package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Spec is one eval task, loaded from tasks/<id>/task.json.
type Spec struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags"`

	TimeoutSec   int     `json:"timeout_sec"`
	MaxBudgetUSD float64 `json:"max_budget_usd"`

	// ExpectedActions is scored as a set (precision/recall) plus first-action
	// match, not as a sequence: several orders are usually valid.
	ExpectedActions  []string `json:"expected_actions"`
	ForbiddenActions []string `json:"forbidden_actions"`

	// ProtectedPaths must be unchanged after the run.
	ProtectedPaths []string `json:"protected_paths"`

	Dir         string `json:"-"`
	Rev         string `json:"-"`
	Instruction string `json:"-"`
}

func (s *Spec) FixtureDir() string   { return filepath.Join(s.Dir, "fixture") }
func (s *Spec) VerifyDir() string    { return filepath.Join(s.Dir, "verify") }
func (s *Spec) SolutionPath() string { return filepath.Join(s.Dir, "solution.sh") }

// EditorVerifyPath is a GDScript snippet run inside the still-open editor,
// for checks against state that doesn't survive the editor closing.
func (s *Spec) EditorVerifyPath() string {
	return filepath.Join(s.VerifyDir(), "editor_verify.gd")
}

func (s *Spec) HasEditorVerify() bool {
	_, err := os.Stat(s.EditorVerifyPath())
	return err == nil
}

// Expects one directory per task under root, each holding a task.json.
func LoadTasks(root string, tags, ids []string) ([]*Spec, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read tasks dir: %w", err)
	}

	wantTags, wantIDs := toSet(tags), toSet(ids)
	allIDs := map[string]bool{}

	var specs []*Spec
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "task.json"))
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		var spec Spec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, fmt.Errorf("%s/task.json: %w", dir, err)
		}
		spec.Dir = dir
		allIDs[spec.ID] = true

		if len(wantIDs) > 0 && !wantIDs[spec.ID] {
			continue
		}
		if len(wantTags) > 0 && !anyIn(spec.Tags, wantTags) {
			continue
		}

		instr, err := os.ReadFile(filepath.Join(dir, "instruction.md"))
		if err != nil {
			return nil, fmt.Errorf("%s: instruction.md: %w", spec.ID, err)
		}
		spec.Instruction = strings.TrimSpace(string(instr))

		if spec.Rev, err = revision(dir); err != nil {
			return nil, err
		}

		spec.applyDefaults()
		specs = append(specs, &spec)
	}

	var unknown []string
	for _, id := range ids {
		if !allIDs[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown task ids: %s", strings.Join(unknown, ", "))
	}

	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	if len(specs) == 0 {
		return nil, fmt.Errorf("no tasks matched (tags=%v ids=%v)", tags, ids)
	}
	return specs, nil
}

func (s *Spec) applyDefaults() {
	if s.TimeoutSec == 0 {
		s.TimeoutSec = 600
	}
	if s.MaxBudgetUSD == 0 {
		s.MaxBudgetUSD = 2.0
	}
}

// revision hashes the task directory, so that editing a task shows up as a new
// revision rather than as a change in the results for the old one.
func revision(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		h.Write([]byte(filepath.ToSlash(rel)))
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

func toSet(xs []string) map[string]bool {
	if len(xs) == 0 {
		return nil
	}
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			m[x] = true
		}
	}
	return m
}

func anyIn(xs []string, set map[string]bool) bool {
	for _, x := range xs {
		if set[x] {
			return true
		}
	}
	return false
}
