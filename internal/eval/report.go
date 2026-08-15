package eval

import (
	"encoding/json"
	"os"
	"time"
)

// Results is one matrix cell's complete output.
type Results struct {
	File string `json:"-"` // where it was written, or read back from

	Model     string     `json:"model"`
	Surface   string     `json:"surface"`
	Repeats   int        `json:"repeats"`
	StartedAt time.Time  `json:"started_at"`
	Attempts  []*Attempt `json:"attempts"`
	Summary   Summary    `json:"summary"`
}

type Summary struct {
	Tasks         int     `json:"tasks"`
	Trials        int     `json:"trials"`
	Passes        int     `json:"passes"`
	PassRate      float64 `json:"pass_rate"`
	PartialCredit float64 `json:"partial_credit"`

	// The gap between PassAllRepeats and PassRate is the flakiness budget: when
	// it's wide, small changes are buried in noise.
	PassAllRepeats float64 `json:"pass_all_repeats"`

	MeanWallSec  float64 `json:"mean_wall_sec"`
	MeanTurns    float64 `json:"mean_turns"`
	MeanTokens   float64 `json:"mean_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`

	MeanFirstActionCorrect float64 `json:"mean_first_action_correct"`
	MeanActionPrecision    float64 `json:"mean_action_precision"`
	MeanActionRecall       float64 `json:"mean_action_recall"`
	MeanBypassRate         float64 `json:"mean_bypass_rate"`
	MeanToolErrorRate      float64 `json:"mean_tool_error_rate"`
	MeanHelpCalls          float64 `json:"mean_help_calls"`

	HarnessErrs int `json:"harness_errors"`
}

func Summarize(attempts []*Attempt) Summary {
	s := Summary{Trials: len(attempts)}
	if len(attempts) == 0 {
		return s
	}

	perTask := map[string][]bool{}
	var wall, turns, tokens, partial float64
	var firstOK, prec, rec, bypass, errRate, help float64
	scored, bypassScored := 0, 0

	for _, a := range attempts {
		perTask[a.TaskID] = append(perTask[a.TaskID], a.Passed)
		if a.Passed {
			s.Passes++
		}
		partial += a.PartialCredit()
		if a.Error != "" {
			s.HarnessErrs++
		}
		if a.Metrics != nil {
			// Not WallTime, which a summary of results read back from JSON
			// would find zeroed.
			wall += float64(a.Metrics.WallTimeMS) / 1000
			turns += float64(a.Metrics.NumTurns)
			tokens += float64(a.Metrics.TotalTokens)
			s.TotalCostUSD += a.Metrics.TotalCostUSD
		}
		if a.Actions.TotalCalls > 0 {
			scored++
			if a.Actions.FirstActionCorrect {
				firstOK++
			}
			prec += a.Actions.Precision
			rec += a.Actions.Recall
			// A 0/0 bypass rate would read as "never bypassed", so an attempt
			// that mutated nothing stays out of the mean entirely.
			if a.Actions.MutatingCalls > 0 {
				bypassScored++
				bypass += a.Actions.BypassRate
			}
			errRate += a.Actions.ErrorRate
			help += float64(a.Actions.HelpCalls)
		}
	}

	n := float64(len(attempts))
	s.Tasks = len(perTask)
	s.PassRate = float64(s.Passes) / n
	s.PartialCredit = partial / n
	s.MeanWallSec = wall / n
	s.MeanTurns = turns / n
	s.MeanTokens = tokens / n

	allPass := 0
	for _, results := range perTask {
		ok := true
		for _, p := range results {
			ok = ok && p
		}
		if ok {
			allPass++
		}
	}
	s.PassAllRepeats = float64(allPass) / float64(len(perTask))

	if scored > 0 {
		f := float64(scored)
		s.MeanFirstActionCorrect = firstOK / f
		s.MeanActionPrecision = prec / f
		s.MeanActionRecall = rec / f
		s.MeanToolErrorRate = errRate / f
		s.MeanHelpCalls = help / f
	}
	if bypassScored > 0 {
		s.MeanBypassRate = bypass / float64(bypassScored)
	}
	return s
}

func WriteJSON(path string, r *Results) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
