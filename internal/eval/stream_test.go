package eval

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// Claude Code can restart the session inside one invocation, emitting a result event per stretch.
func TestParseStreamSumsEverySegment(t *testing.T) {
	is := is.New(t)

	stream := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s1","model":"haiku"}`,
		`{"type":"result","subtype":"success","num_turns":5,"duration_ms":1000,"total_cost_usd":0.04,` +
			`"usage":{"input_tokens":100,"output_tokens":10}}`,
		`{"type":"system","subtype":"init","session_id":"s1","model":"haiku"}`,
		`{"type":"result","subtype":"success","num_turns":1,"duration_ms":500,"total_cost_usd":0.09,` +
			`"usage":{"input_tokens":20,"output_tokens":3}}`,
	}, "\n")

	m, err := ParseStream(strings.NewReader(stream))
	is.NoErr(err)

	is.Equal(m.Segments, 2)
	is.Equal(m.NumTurns, 6)
	is.Equal(m.TotalCostUSD, 0.13)
	is.Equal(m.AgentTimeMS, int64(1500))
	is.Equal(m.TotalTokens, 133)
}
