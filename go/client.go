package kimi

import (
	"context"
	"encoding/json"
	"sync"
)

const protocolVersion = "1.0"

type Client struct {
	rpc         *rpcClient
	mu          sync.Mutex
	sessions    map[string]*Session
	listeners   map[int]func(Event)
	nextSubID   int
	unsubscribe func()
}

func Connect(ctx context.Context, opts ...ClientOption) (*Client, error) {
	options := ClientOptions{
		Executable: "kimi-code",
		Args:       []string{"sdk-server", "--stdio"},
		ClientName: "kimi-code-agent-sdk-go",
	}
	for _, opt := range opts {
		opt(&options)
	}
	rpc, err := newRPCClient(ctx, options.Executable, options.Args, options.Cwd, options.Env)
	if err != nil {
		return nil, err
	}
	client := &Client{
		rpc:       rpc,
		sessions:  make(map[string]*Session),
		listeners: make(map[int]func(Event)),
	}
	client.unsubscribe = rpc.onNotification(client.handleNotification)
	if err := client.initialize(ctx, options.ClientName, options.ClientVersion); err != nil {
		_ = client.Close(ctx)
		return nil, err
	}
	return client, nil
}

func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	return Connect(ctx, opts...)
}

func (c *Client) initialize(ctx context.Context, clientName string, clientVersion string) error {
	_, err := c.rpc.request(ctx, "initialize", map[string]any{
		"supportedVersions": []string{protocolVersion},
		"client": map[string]any{
			"name":    clientName,
			"version": clientVersion,
		},
	})
	return err
}

func (c *Client) CreateSession(ctx context.Context, opts ...SessionOption) (*Session, error) {
	options := SessionOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	params := map[string]any{"workDir": options.WorkDir}
	if options.ID != "" {
		params["id"] = options.ID
	}
	if options.Model != "" {
		params["model"] = options.Model
	}
	if options.Thinking != nil {
		params["thinking"] = options.Thinking
	}
	if options.Permission != "" {
		params["permission"] = options.Permission
	}
	if options.Metadata != nil {
		params["metadata"] = options.Metadata
	}
	raw, err := c.rpc.request(ctx, "createSession", params)
	if err != nil {
		return nil, err
	}
	var summary SessionSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, transportError("session summary must be valid JSON.", err)
	}
	return c.bindSession(summary), nil
}

func (c *Client) ResumeSession(ctx context.Context, id string) (*Session, error) {
	raw, err := c.rpc.request(ctx, "resumeSession", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	var summary SessionSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, transportError("session summary must be valid JSON.", err)
	}
	return c.bindSession(summary), nil
}

func (c *Client) ListSessions(ctx context.Context, opts ...ListSessionsOption) ([]SessionSummary, error) {
	options := ListSessionsOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	params := map[string]any{}
	if options.WorkDir != "" {
		params["workDir"] = options.WorkDir
	}
	if options.SessionID != "" {
		params["sessionId"] = options.SessionID
	}
	raw, err := c.rpc.request(ctx, "listSessions", params)
	if err != nil {
		return nil, err
	}
	var summaries []SessionSummary
	if err := json.Unmarshal(raw, &summaries); err != nil {
		return nil, transportError("listSessions result must be valid JSON.", err)
	}
	return summaries, nil
}

func (c *Client) Close(ctx context.Context) error {
	if c.unsubscribe != nil {
		c.unsubscribe()
	}
	_, _ = c.rpc.request(ctx, "shutdown", map[string]any{})
	return c.rpc.close()
}

func (c *Client) CloseSession(ctx context.Context, sessionID string) error {
	_, err := c.rpc.request(ctx, "closeSession", map[string]any{"sessionId": sessionID})
	c.mu.Lock()
	delete(c.sessions, sessionID)
	c.mu.Unlock()
	return err
}

func (c *Client) OnEvent(listener func(Event)) func() {
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

func (c *Client) Prompt(ctx context.Context, sessionID string, input PromptInput) (<-chan Event, error) {
	return c.streamTurn(ctx, sessionID, "prompt", map[string]any{"sessionId": sessionID, "input": input})
}

func (c *Client) Steer(ctx context.Context, sessionID string, input PromptInput) error {
	_, err := c.rpc.request(ctx, "steer", map[string]any{"sessionId": sessionID, "input": input})
	return err
}

func (c *Client) Cancel(ctx context.Context, sessionID string, turnID *int) error {
	params := map[string]any{"sessionId": sessionID}
	if turnID != nil {
		params["turnId"] = *turnID
	}
	_, err := c.rpc.request(ctx, "cancel", params)
	return err
}

func (c *Client) GetStatus(ctx context.Context, sessionID string) (SessionStatus, error) {
	raw, err := c.rpc.request(ctx, "getStatus", map[string]any{"sessionId": sessionID})
	if err != nil {
		return SessionStatus{}, err
	}
	var status SessionStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return SessionStatus{}, transportError("session status must be valid JSON.", err)
	}
	return status, nil
}

func (c *Client) bindSession(summary SessionSummary) *Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	if session, ok := c.sessions[summary.ID]; ok {
		return session
	}
	session := &Session{client: c, summary: summary}
	c.sessions[summary.ID] = session
	return session
}

func (c *Client) handleNotification(method string, params json.RawMessage) {
	if method != "event" {
		return
	}
	var event Event
	if err := json.Unmarshal(params, &event); err != nil {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(params, &raw); err == nil {
		event.Raw = raw
	}
	if event.Type == "" || event.SessionID == "" || event.AgentID == "" {
		return
	}
	c.mu.Lock()
	listeners := make([]func(Event), 0, len(c.listeners))
	for _, listener := range c.listeners {
		listeners = append(listeners, listener)
	}
	c.mu.Unlock()
	for _, listener := range listeners {
		listener(event)
	}
}

func (c *Client) streamTurn(
	ctx context.Context,
	sessionID string,
	method string,
	params map[string]any,
) (<-chan Event, error) {
	events := make(chan Event, 128)
	buffer := make(chan Event, 128)
	unsubscribe := c.OnEvent(func(event Event) {
		if event.SessionID == sessionID {
			buffer <- event
		}
	})
	raw, err := c.rpc.request(ctx, method, params)
	if err != nil {
		unsubscribe()
		close(events)
		return nil, err
	}
	var result struct {
		TurnID int `json:"turnId"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		unsubscribe()
		close(events)
		return nil, transportError("prompt result must include turnId.", err)
	}
	go func() {
		defer unsubscribe()
		defer close(events)
		for {
			select {
			case event := <-buffer:
				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
				if event.Type == "turn.ended" && event.TurnID != nil && *event.TurnID == result.TurnID {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

type Session struct {
	client  *Client
	summary SessionSummary
}

func (s *Session) ID() string {
	return s.summary.ID
}

func (s *Session) Summary() SessionSummary {
	return s.summary
}

func (s *Session) Prompt(ctx context.Context, input PromptInput) (<-chan Event, error) {
	return s.client.Prompt(ctx, s.summary.ID, input)
}

func (s *Session) Close(ctx context.Context) error {
	return s.client.CloseSession(ctx, s.summary.ID)
}

func (s *Session) Steer(ctx context.Context, input PromptInput) error {
	return s.client.Steer(ctx, s.summary.ID, input)
}

func (s *Session) Cancel(ctx context.Context, turnID *int) error {
	return s.client.Cancel(ctx, s.summary.ID, turnID)
}

func (s *Session) GetStatus(ctx context.Context) (SessionStatus, error) {
	return s.client.GetStatus(ctx, s.summary.ID)
}
