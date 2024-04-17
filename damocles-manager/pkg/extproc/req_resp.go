package extproc

import (
	"encoding/json"
	"fmt"
)

type Request struct {
	ID   uint64          `json:"id"`
	Data json.RawMessage `json:"task"`
}

func (r *Request) SetData(data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	r.Data = b
	return nil
}

func (r *Request) DecodeInto(v any) error {
	err := json.Unmarshal(r.Data, v)
	if err != nil {
		return fmt.Errorf("json unmarshal: %w", err)
	}

	return nil
}

type Response struct {
	ID     uint64          `json:"id"`
	ErrMsg *string         `json:"err_msg"`
	Result json.RawMessage `json:"output"`
}

func (r *Response) SetResult(res any) {
	b, err := json.Marshal(res)
	if err != nil {
		errMsg := err.Error()
		r.ErrMsg = &errMsg
		return
	}

	r.Result = b
}

func (r *Response) DecodeInto(v any) error {
	err := json.Unmarshal(r.Result, v)
	if err != nil {
		return fmt.Errorf("json unmarshal: %w", err)
	}

	return nil
}
