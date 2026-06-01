package kimi

type PermissionMode string

const (
	PermissionManual PermissionMode = "manual"
	PermissionAuto   PermissionMode = "auto"
	PermissionYolo   PermissionMode = "yolo"
)

type PromptInput any

type PromptPart map[string]any

type ClientOptions struct {
	Executable    string
	Args          []string
	Cwd           string
	Env           []string
	ClientName    string
	ClientVersion string
}

type ClientOption func(*ClientOptions)

func WithExecutable(executable string) ClientOption {
	return func(options *ClientOptions) { options.Executable = executable }
}

func WithArgs(args ...string) ClientOption {
	return func(options *ClientOptions) { options.Args = append([]string(nil), args...) }
}

func WithCwd(cwd string) ClientOption {
	return func(options *ClientOptions) { options.Cwd = cwd }
}

func WithEnv(env []string) ClientOption {
	return func(options *ClientOptions) { options.Env = append([]string(nil), env...) }
}

func WithClientName(name string) ClientOption {
	return func(options *ClientOptions) { options.ClientName = name }
}

func WithClientVersion(version string) ClientOption {
	return func(options *ClientOptions) { options.ClientVersion = version }
}

type SessionOptions struct {
	ID         string
	WorkDir    string
	Model      string
	Thinking   any
	Permission PermissionMode
	Metadata   map[string]any
}

type SessionOption func(*SessionOptions)

func WithSessionID(id string) SessionOption {
	return func(options *SessionOptions) { options.ID = id }
}

func WithWorkDir(workDir string) SessionOption {
	return func(options *SessionOptions) { options.WorkDir = workDir }
}

func WithModel(model string) SessionOption {
	return func(options *SessionOptions) { options.Model = model }
}

func WithThinking(thinking any) SessionOption {
	return func(options *SessionOptions) { options.Thinking = thinking }
}

func WithPermission(permission PermissionMode) SessionOption {
	return func(options *SessionOptions) { options.Permission = permission }
}

func WithMetadata(metadata map[string]any) SessionOption {
	return func(options *SessionOptions) { options.Metadata = metadata }
}

type ListSessionsOptions struct {
	WorkDir   string
	SessionID string
}

type ListSessionsOption func(*ListSessionsOptions)

func WithListWorkDir(workDir string) ListSessionsOption {
	return func(options *ListSessionsOptions) { options.WorkDir = workDir }
}

func WithListSessionID(sessionID string) ListSessionsOption {
	return func(options *ListSessionsOptions) { options.SessionID = sessionID }
}

type SessionSummary struct {
	ID         string         `json:"id"`
	Title      string         `json:"title,omitempty"`
	LastPrompt string         `json:"lastPrompt,omitempty"`
	WorkDir    string         `json:"workDir"`
	SessionDir string         `json:"sessionDir"`
	CreatedAt  float64        `json:"createdAt"`
	UpdatedAt  float64        `json:"updatedAt"`
	Archived   bool           `json:"archived,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type SessionStatus struct {
	Model            string         `json:"model,omitempty"`
	ThinkingLevel    string         `json:"thinkingLevel"`
	Permission       PermissionMode `json:"permission"`
	PlanMode         bool           `json:"planMode"`
	ContextTokens    float64        `json:"contextTokens"`
	MaxContextTokens float64        `json:"maxContextTokens"`
	ContextUsage     float64        `json:"contextUsage"`
	Usage            any            `json:"usage,omitempty"`
}

type Event struct {
	Type      string         `json:"type"`
	SessionID string         `json:"sessionId"`
	AgentID   string         `json:"agentId"`
	TurnID    *int           `json:"turnId,omitempty"`
	Delta     string         `json:"delta,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Error     any            `json:"error,omitempty"`
	Raw       map[string]any `json:"-"`
}
