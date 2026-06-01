package kimi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"sync"
)

type rpcClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	writeMu   sync.Mutex
	mu        sync.Mutex
	nextID    int
	closed    bool
	pending   map[string]chan rpcResult
	listeners map[int]func(string, json.RawMessage)
	nextSubID int
	waitDone  chan error
}

type rpcResult struct {
	result json.RawMessage
	err    error
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *string         `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Data    *rpcErrorData `json:"data,omitempty"`
}

type rpcErrorData struct {
	Retryable bool `json:"retryable,omitempty"`
	Details   any  `json:"details,omitempty"`
}

func newRPCClient(ctx context.Context, executable string, args []string, cwd string, env []string) (*rpcClient, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if env != nil {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	client := &rpcClient{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		nextID:    1,
		pending:   make(map[string]chan rpcResult),
		listeners: make(map[int]func(string, json.RawMessage)),
		waitDone:  make(chan error, 1),
	}
	go io.Copy(io.Discard, stderr)
	go client.readLoop()
	go client.waitLoop()
	return client, nil
}

func (c *rpcClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, transportError("Kimi Code SDK server is closed.", nil)
	}
	id := strconv.Itoa(c.nextID)
	c.nextID++
	resultCh := make(chan rpcResult, 1)
	c.pending[id] = resultCh
	c.mu.Unlock()

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	_, writeErr := c.stdin.Write(append(payload, '\n'))
	c.writeMu.Unlock()
	if writeErr != nil {
		c.removePending(id)
		return nil, transportError(writeErr.Error(), writeErr)
	}

	select {
	case result := <-resultCh:
		return result.result, result.err
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	}
}

func (c *rpcClient) onNotification(listener func(string, json.RawMessage)) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextSubID
	c.nextSubID++
	c.listeners[id] = listener
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.listeners, id)
	}
}

func (c *rpcClient) close() error {
	c.closeWithError(transportError("Kimi Code SDK server is closed.", nil))
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return <-c.waitDone
}

func (c *rpcClient) readLoop() {
	reader := bufio.NewReader(c.stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.closeWithError(transportError(err.Error(), err))
			} else {
				c.closeWithError(transportError("Kimi Code SDK server exited.", nil))
			}
			return
		}
		c.handleLine(line)
	}
}

func (c *rpcClient) waitLoop() {
	err := c.cmd.Wait()
	if err != nil {
		c.closeWithError(transportError("Kimi Code SDK server exited.", err))
	}
	c.waitDone <- err
}

func (c *rpcClient) handleLine(line []byte) {
	var response rpcResponse
	if err := json.Unmarshal(line, &response); err != nil {
		c.closeWithError(protocolError("Malformed JSON-RPC response.", "INVALID_REQUEST", false, err))
		return
	}
	if response.Method != "" && response.ID == nil {
		c.dispatchNotification(response.Method, response.Params)
		return
	}
	if response.ID == nil {
		return
	}
	ch := c.removePending(*response.ID)
	if ch == nil {
		return
	}
	if response.Error != nil {
		retryable := false
		var details any
		if response.Error.Data != nil {
			retryable = response.Error.Data.Retryable
			details = response.Error.Data.Details
		}
		ch <- rpcResult{err: protocolError(response.Error.Message, response.Error.Code, retryable, details)}
		return
	}
	ch <- rpcResult{result: response.Result}
}

func (c *rpcClient) dispatchNotification(method string, params json.RawMessage) {
	c.mu.Lock()
	listeners := make([]func(string, json.RawMessage), 0, len(c.listeners))
	for _, listener := range c.listeners {
		listeners = append(listeners, listener)
	}
	c.mu.Unlock()
	for _, listener := range listeners {
		listener(method, params)
	}
}

func (c *rpcClient) closeWithError(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	pending := c.pending
	c.pending = make(map[string]chan rpcResult)
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- rpcResult{err: err}
	}
}

func (c *rpcClient) removePending(id string) chan rpcResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := c.pending[id]
	delete(c.pending, id)
	return ch
}
