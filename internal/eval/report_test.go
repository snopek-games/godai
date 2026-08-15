package eval

import (
	"testing"

	"github.com/matryer/is"
)

func TestMeanBypassRateSkipsAttemptsThatMutatedNothing(t *testing.T) {
	is := is.New(t)

	s := Summarize([]*Attempt{
		{Actions: ActionStats{TotalCalls: 4}},
		{Actions: ActionStats{TotalCalls: 3, MutatingCalls: 2, BypassRate: 0}},
		{Actions: ActionStats{TotalCalls: 2, MutatingCalls: 1, BypassRate: 1}},
	})
	is.Equal(s.MeanBypassRate, 0.5)
}
