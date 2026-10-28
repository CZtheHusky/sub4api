package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
)

// One downstream reader survives turn boundaries so an interrupt can arrive
// while the upstream is generating, without losing a queued next create.
type openAIWSIngressControl struct {
	ctx         context.Context
	conn        *coderws.Conn
	idleTimeout time.Duration
	idle        atomic.Bool
	idleStart   chan struct{}
	reading     chan openAIWSClientReadResult
	queued      *openAIWSClientReadResult
	interruptID string
}

func (r *openAIWSIngressControl) startRead() {
	if r.reading != nil || r.queued != nil {
		return
	}
	r.reading = make(chan openAIWSClientReadResult, 1)
	result := r.reading
	go func() {
		kind, body, err := readOpenAIWSClientMessageWithTimeoutStart(r.ctx, r.conn, r.idleTimeout,
			coderws.StatusNormalClosure, "websocket idle timeout", r.idleStart, r.idle.Load)
		result <- openAIWSClientReadResult{messageType: kind, payload: body, err: err}
	}()
}

func (r *openAIWSIngressControl) readClient() (coderws.MessageType, []byte, error) {
	r.idle.Store(true)
	r.startRead()
	select {
	case r.idleStart <- struct{}{}:
	default:
	}
	var result openAIWSClientReadResult
	if r.queued != nil {
		result = *r.queued
		r.queued = nil
	} else {
		result = <-r.reading
		r.reading = nil
	}
	r.idle.Store(false)
	return result.messageType, result.payload, result.err
}

func (r *openAIWSIngressControl) readUpstream(lease *openAIWSConnLease, timeout, writeTimeout time.Duration) ([]byte, error) {
	r.idle.Store(false)
	r.startRead()
	readCtx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	upstream := make(chan openAIWSClientReadResult, 1)
	go func() {
		body, err := lease.ReadMessageWithContextTimeout(readCtx, timeout)
		upstream <- openAIWSClientReadResult{payload: body, err: err}
	}()
	for {
		select {
		case result := <-upstream:
			kind := gjson.GetBytes(result.payload, "type").String()
			if (kind == "response.interrupt.accepted" || kind == "response.interrupt.failed") &&
				gjson.GetBytes(result.payload, "response_id").String() == r.interruptID {
				r.interruptID = ""
			}
			return result.payload, result.err
		case result := <-r.reading:
			r.reading = nil
			if result.err != nil || gjson.GetBytes(result.payload, "type").String() != "response.interrupt" {
				r.queued = &result
				continue
			}
			if err := lease.WriteJSONWithContextTimeout(r.ctx, json.RawMessage(result.payload), writeTimeout); err != nil {
				cancel()
				<-upstream
				return nil, err
			}
			r.interruptID = gjson.GetBytes(result.payload, "response_id").String()
			r.startRead()
		}
	}
}

// HTTP has no response.interrupt operation. Report that capability honestly
// while draining the HTTP turn, so its completed items and usage are retained.
func (r *openAIWSIngressControl) relayHTTP(
	run func(func([]byte) error) (*OpenAIForwardResult, error),
	write func([]byte) error,
	rejectInterrupt func([]byte) error,
) (*OpenAIForwardResult, error) {
	type result struct {
		forward *OpenAIForwardResult
		err     error
	}
	completed := make(chan result, 1)
	messages := make(chan []byte)
	written := make(chan error)
	go func() {
		forward, err := run(func(message []byte) error {
			messages <- message
			return <-written
		})
		completed <- result{forward, err}
	}()
	for {
		select {
		case result := <-completed:
			return result.forward, result.err
		case message := <-messages:
			err := write(message)
			written <- err
			if err == nil {
				r.startRead()
			}
		case result := <-r.reading:
			r.reading = nil
			if result.err != nil || gjson.GetBytes(result.payload, "type").String() != "response.interrupt" {
				r.queued = &result
				continue
			}
			if err := rejectInterrupt(result.payload); err != nil {
				r.queued = &openAIWSClientReadResult{err: err}
			} else {
				r.startRead()
			}
		}
	}
}

func (r *openAIWSIngressControl) close(err error) {
	if r.reading == nil {
		return
	}
	status, reason := coderws.StatusNormalClosure, "websocket session finished"
	if err != nil && !isOpenAIWSClientDisconnectError(err) {
		status, reason = coderws.StatusInternalError, "upstream websocket proxy failed"
	}
	var closeErr *OpenAIWSClientCloseError
	if errors.As(err, &closeErr) {
		status, reason = closeErr.StatusCode(), closeErr.Reason()
	}
	_ = r.conn.Close(status, reason)
	_ = r.conn.CloseNow()
	<-r.reading
	r.reading = nil
}
