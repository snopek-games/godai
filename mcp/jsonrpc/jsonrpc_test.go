package jsonrpc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func normalizeJSON(s string) (string, error) {
	if len(s) == 0 {
		return "", nil
	}

	var v any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func TestSpec(t *testing.T) {
	// These are taken from the JSONRPC v2 spec:
	//   https://www.jsonrpc.org/specification
	cases := []struct {
		name   string
		input  string
		output string
	}{
		{
			"PositionalParams1",
			`{"jsonrpc": "2.0", "method": "subtract", "params": [42, 23], "id": 1}`,
			`{"jsonrpc": "2.0", "result": 19, "id": 1}`,
		},
		{
			"PositionalParams2",
			`{"jsonrpc": "2.0", "method": "subtract", "params": [23, 42], "id": 2}`,
			`{"jsonrpc": "2.0", "result": -19, "id": 2}`,
		},
		{
			"NamedParams1",
			`{"jsonrpc": "2.0", "method": "subtract", "params": {"subtrahend": 23, "minuend": 42}, "id": 3}`,
			`{"jsonrpc": "2.0", "result": 19, "id": 3}`,
		},
		{
			"NamedParams2",
			`{"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4}`,
			`{"jsonrpc": "2.0", "result": 19, "id": 4}`,
		},
		{
			"Notification1",
			`{"jsonrpc": "2.0", "method": "update", "params": [1,2,3,4,5]}`,
			"",
		},
		{
			"Notification2",
			`{"jsonrpc": "2.0", "method": "foobar"}`,
			"",
		},
		{
			"NonExistentMethod",
			`{"jsonrpc": "2.0", "method": "foobar", "id": "1"}`,
			`{"jsonrpc": "2.0", "error": {"code": -32601, "message": "Method not found"}, "id": "1"}`,
		},
		{
			"InvalidJSON",
			`{"jsonrpc": "2.0", "method": "foobar, "params": "bar", "baz]`,
			`{"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}, "id": null}`,
		},
		{
			"InvalidRequest",
			`{"jsonrpc": "2.0", "method": 1, "params": "bar"}`,
			`{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}`,
		},
		{
			"BatchInvalidJSON",
			`[
				{"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"},
				{"jsonrpc": "2.0", "method"
			]`,
			`{"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}, "id": null}`,
		},
		{
			"BatchEmptyArray",
			`[]`,
			`{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}`,
		},
		{
			"BatchInvalidRequest1",
			`[1]`,
			`[
				{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}
			]`,
		},
		{
			"BatchInvalidRequest2",
			`[1,2,3]`,
			`[
				{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null},
				{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null},
				{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}
			]`,
		},
		{
			"BatchNormal",
			`[
				{"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"},
				{"jsonrpc": "2.0", "method": "notify_hello", "params": [7]},
				{"jsonrpc": "2.0", "method": "subtract", "params": [42,23], "id": "2"},
				{"foo": "boo"},
				{"jsonrpc": "2.0", "method": "foo.get", "params": {"name": "myself"}, "id": "5"},
				{"jsonrpc": "2.0", "method": "get_data", "id": "9"}
			]`,
			`[
				{"jsonrpc": "2.0", "result": 7, "id": "1"},
				{"jsonrpc": "2.0", "result": 19, "id": "2"},
				{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null},
				{"jsonrpc": "2.0", "error": {"code": -32601, "message": "Method not found"}, "id": "5"},
				{"jsonrpc": "2.0", "result": ["hello", 5], "id": "9"}
			]`,
		},
		{
			"BatchAllNotifications",
			`[
				{"jsonrpc": "2.0", "method": "sum", "params": [1,2,4]},
				{"jsonrpc": "2.0", "method": "notify_hello", "params": [7]}
			]`,
			"",
		},
	}

	d := NewDispatcher()

	d.Register("subtract", func(ctx context.Context, rawParams json.RawMessage) (any, *Error) {
		if rawParams[0] == '[' {
			var nums []int
			if err := json.Unmarshal(rawParams, &nums); err != nil {
				return nil, NewError(InvalidParamsErrorCode, "Invalid params", nil)
			}
			if len(nums) < 2 {
				return nil, NewError(InvalidParamsErrorCode, "Must have at least 2 params", nil)
			}

			result := nums[0]
			for _, num := range nums[1:] {
				result -= num
			}
			return result, nil
		} else if rawParams[0] == '{' {
			var params struct {
				Subtrahend int `json:"subtrahend"`
				Minuend    int `json:"minuend"`
			}
			if err := json.Unmarshal(rawParams, &params); err != nil {
				return nil, NewError(InvalidParamsErrorCode, "Invalid params", nil)
			}

			var result int = params.Minuend - params.Subtrahend
			return result, nil
		}

		return nil, NewError(InvalidParamsErrorCode, "Invalid params", nil)
	})

	d.Register("sum", func(ctx context.Context, rawParams json.RawMessage) (any, *Error) {
		var nums []int
		if err := json.Unmarshal(rawParams, &nums); err != nil {
			return nil, NewError(InvalidParamsErrorCode, "Invalid params", nil)
		}
		if len(nums) < 2 {
			return nil, NewError(InvalidParamsErrorCode, "Must have at least 2 params", nil)
		}

		result := nums[0]
		for _, num := range nums[1:] {
			result += num
		}
		return result, nil
	})

	d.Register("notify_hello", func(ctx context.Context, rawParams json.RawMessage) (any, *Error) {
		return "hello", nil
	})

	d.Register("get_data", func(ctx context.Context, rawParams json.RawMessage) (any, *Error) {
		return []any{"hello", 5}, nil
	})

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			ctx := context.TODO()

			resp, err := d.Handle(ctx, []byte(tc.input))
			is.NoErr(err)

			normalizedResult, err := normalizeJSON(string(resp))
			is.NoErr(err)

			normalizedExpected, err := normalizeJSON(tc.output)
			is.NoErr(err)

			is.Equal(normalizedResult, normalizedExpected)
		})
	}
}
