package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

func NewRequest(id string, method string, params json.RawMessage) *Request {
	return &Request{
		JSONRPC: "2.0",
		ID:      jsonMarshalUnchecked(id),
		Method:  method,
		Params:  params,
	}
}

func NewNotification(method string, params json.RawMessage) *Request {
	return &Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
}

func (r *Request) SetStringID(id string) {
	r.ID = jsonMarshalUnchecked(id)
}

func (r *Request) GetStringID() (string, bool) {
	if len(r.ID) == 0 {
		return "", false
	}
	var idStr string
	if err := json.Unmarshal(r.ID, &idStr); err != nil {
		return "", false
	}
	return idStr, true
}

func (r *Request) IsValid() bool {
	return r.JSONRPC == "2.0" && r.Method != ""
}

func (r *Request) HasID() bool {
	return len(r.ID) > 0
}

func (r *Request) SetParams(data any) {
	r.Params = jsonMarshalUnchecked(data)
}

type ErrorCode int

const (
	ParseErrorCode          ErrorCode = -32700
	InvalidRequestErrorCode           = -32600
	MethodNotFoundErrorCode           = -32601
	InvalidParamsErrorCode            = -32602
	InternalErrorCode                 = -32603
	ServerErrorMinCode                = -32000
	ServerErrorMaxCode                = -32099
)

type Error struct {
	Code    ErrorCode       `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

func (r *Response) GetID() (string, bool) {
	if len(r.ID) == 0 {
		return "", false
	}
	var idStr string
	if err := json.Unmarshal(r.ID, &idStr); err != nil {
		return "", false
	}
	return idStr, true
}

func (r *Response) IsValid() bool {
	return r.JSONRPC == "2.0" && len(bytes.TrimSpace(r.ID)) > 0
}

func jsonMarshalUnchecked(data any) []byte {
	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return b
}

func NewError(code ErrorCode, msg string, data any) *Error {
	var jsonData json.RawMessage
	if data != nil {
		jsonData = json.RawMessage(jsonMarshalUnchecked(data))
	}
	return &Error{
		Code:    code,
		Message: msg,
		Data:    jsonData,
	}
}

func NewResponse(id json.RawMessage) *Response {
	if len(id) == 0 {
		id = NullID()
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
	}
}

func (r *Response) SetResult(data any) {
	r.Result = jsonMarshalUnchecked(data)
}

func NewErrorResponse(id json.RawMessage, rpcError *Error) *Response {
	resp := NewResponse(id)
	resp.Error = rpcError
	return resp
}

func NullID() json.RawMessage {
	return json.RawMessage("null")
}

var invalidRequest Request = Request{
	JSONRPC: "Invalid",
	ID:      NullID(),
}

func ParseRequests(b []byte) ([]Request, bool, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, false, fmt.Errorf("empty JSON")
	}

	switch b[0] {
	case '[':
		var batch []json.RawMessage
		if err := json.Unmarshal(b, &batch); err != nil {
			return nil, true, err
		}

		var requests []Request
		for _, req_b := range batch {
			dec := json.NewDecoder(bytes.NewReader(req_b))
			dec.UseNumber()

			var req Request
			if err := dec.Decode(&req); err != nil {
				if _, ok := err.(*json.SyntaxError); ok {
					return nil, true, err
				}
				req = invalidRequest
			}

			requests = append(requests, req)
		}
		return requests, true, nil
	case '{':
		var req Request

		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()

		if err := dec.Decode(&req); err != nil {
			if _, ok := err.(*json.SyntaxError); ok {
				return nil, false, err
			}
			req = invalidRequest
		}
		return []Request{req}, false, nil
	default:
		return nil, false, fmt.Errorf("unexpected top-level JSON: %q", b[0])
	}
}
