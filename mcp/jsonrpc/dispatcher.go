package jsonrpc

import (
	"context"
	"encoding/json"
)

func newParseError() *Error {
	return &Error{
		Code:    ParseErrorCode,
		Message: "Parse error",
	}
}

func newInvalidRequestError() *Error {
	return &Error{
		Code:    InvalidRequestErrorCode,
		Message: "Invalid request",
	}
}

type Handler func(ctx context.Context, params json.RawMessage) (result any, rpcErr *Error)

type Dispatcher struct {
	handlers map[string]Handler
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]Handler),
	}
}

func (d *Dispatcher) Register(method string, h Handler) {
	d.handlers[method] = h
}

func (d *Dispatcher) Handle(ctx context.Context, data []byte) ([]byte, error) {
	requests, isBatch, err := ParseRequests(data)

	if err != nil {
		return json.Marshal(NewErrorResponse(NullID(), newParseError()))
	}
	if len(requests) == 0 {
		return json.Marshal(NewErrorResponse(NullID(), newInvalidRequestError()))
	}

	var responses []*Response

	for _, req := range requests {
		if !req.IsValidRequest() {
			responses = append(responses, NewErrorResponse(NullID(), newInvalidRequestError()))
		} else {
			resp := d.handleOne(ctx, req)
			if req.HasValidId() {
				responses = append(responses, resp)
			}
		}
	}

	if len(responses) == 0 {
		return []byte{}, nil
	}

	if isBatch {
		return json.Marshal(responses)
	}

	return json.Marshal(responses[0])
}

func (d *Dispatcher) handleOne(ctx context.Context, req Request) *Response {
	id := req.ID
	if len(id) == 0 {
		id = NullID()
	}

	resp := &Response{JSONRPC: "2.0", ID: id}

	h, ok := d.handlers[req.Method]
	if !ok {
		resp.Error = NewError(MethodNotFoundErrorCode, "Method not found", nil)
		return resp
	}

	result, rpcErr := h(ctx, req.Params)
	if rpcErr != nil {
		resp.Error = rpcErr
		return resp
	}

	resp.Result = result
	return resp
}
