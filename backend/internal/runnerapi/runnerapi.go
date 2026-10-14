// Package runnerapi holds the JSON bodies of the runner API (RFC-108). The
// control plane and the runner both use them; neither imports the other.
package runnerapi

import (
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/providers"
)

// Protocol is the runner API version of this binary (RFC-109).
const Protocol = 1

type Register struct {
	Token    string   `json:"token"`
	Name     string   `json:"name"`
	Labels   []string `json:"labels"`
	Slots    int      `json:"slots"`
	Version  string   `json:"version"`
	Protocol int      `json:"protocol"`
}

type Registered struct {
	RunnerID         string `json:"runnerId"`
	Secret           string `json:"secret"`
	HeartbeatSeconds int    `json:"heartbeatSeconds"`
	Protocol         int    `json:"protocol"`
}

type Heartbeat struct {
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
	// Agents are the engines the runner can drive: builtin, and claude_code
	// when its Claude Code binary answers --version (RFC-121).
	Agents []string `json:"agents,omitempty"`
}

type HeartbeatReply struct {
	State    string   `json:"state"`
	Protocol int      `json:"protocol"`
	Leases   []string `json:"leases"`
}

type Claim struct {
	WaitSeconds int `json:"waitSeconds"`
}

type Claimed struct {
	LeaseID   string `json:"leaseId"`
	CardID    string `json:"cardId"`
	ProjectID string `json:"projectId"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type MCP struct {
	Scope    string   `json:"scope"`
	Offered  []string `json:"offered"`
	Missing  []string `json:"missing"`
	Attached []string `json:"attached"`
	Detached []string `json:"detached"`
	Reason   string   `json:"reason,omitempty"`
	Total    int      `json:"total"`
}

type Resume struct {
	AttemptID string `json:"attemptId"`
	SHA       string `json:"sha"`
}

// Job is what start returns: the job context, or Done with the card status
// when the control plane finished the job itself.
type Job struct {
	Done               bool     `json:"done,omitempty"`
	CardID             string   `json:"cardId"`
	ProjectID          string   `json:"projectId"`
	Title              string   `json:"title,omitempty"`
	Status             string   `json:"status"`
	BaseBranch         string   `json:"baseBranch,omitempty"`
	CloneURL           string   `json:"cloneUrl,omitempty"`
	CloneToken         string   `json:"cloneToken,omitempty"`
	Author             Author   `json:"author"`
	PublishedBranch    bool     `json:"publishedBranch"`
	Fixture            bool     `json:"fixture"`
	Model              string   `json:"model,omitempty"`
	TaskClass          string   `json:"taskClass,omitempty"`
	MaxTurns           int      `json:"maxTurns,omitempty"`
	HistoryBudget      int      `json:"historyBudget,omitempty"`
	MaxTokensPerCard   int      `json:"maxTokensPerCard,omitempty"`
	WallMinutes        int      `json:"wallMinutes,omitempty"`
	LocalCheck         string   `json:"localCheck,omitempty"`
	AllowExecutables   []string `json:"allowExecutables,omitempty"`
	CommandExecutables []string `json:"commandExecutables,omitempty"`
	SandboxEngine      string   `json:"sandboxEngine,omitempty"`
	SandboxImage       string   `json:"sandboxImage,omitempty"`
	MCP                *MCP     `json:"mcp,omitempty"`
	Resume             *Resume  `json:"resume,omitempty"`
	// QuestionMode and QuestionRounds are the card's answer mode and its
	// attempts that ended with a question (RFC-114, RFC-115).
	QuestionMode   string `json:"questionMode,omitempty"`
	QuestionRounds int    `json:"questionRounds,omitempty"`
	// CardBundle means the control plane holds orch/<card> ahead of the base
	// and the card has no pull request: the runner seeds the branch from
	// GET .../bundle (RFC-119). ContextTokens is the routed model's context
	// window (RFC-117).
	CardBundle    bool `json:"cardBundle"`
	ContextTokens int  `json:"contextTokens,omitempty"`
	// Agent is the company's engine; ClaudeCode is set when it is claude_code
	// and carries the family key, which the runner never writes to disk
	// (RFC-122).
	Agent      string         `json:"agent,omitempty"`
	ClaudeCode *ClaudeCodeJob `json:"claudeCode,omitempty"`
}

type ClaudeCodeJob struct {
	BaseURL      string `json:"baseUrl,omitempty"`
	APIKey       string `json:"apiKey"`
	Auth         string `json:"auth"`
	Model        string `json:"model"`
	SmallModel   string `json:"smallModel,omitempty"`
	MaxTurns     int    `json:"maxTurns,omitempty"`
	AllowWeb     bool   `json:"allowWeb"`
	Instructions string `json:"instructions,omitempty"`
}

// BoardOp is one board call of an attempt. Only the fields of Op are read.
type BoardOp struct {
	Op        string         `json:"op"`
	Holder    string         `json:"holder,omitempty"`
	IDs       []string       `json:"ids,omitempty"`
	AttemptID string         `json:"attemptId,omitempty"`
	Attempt   *Attempt       `json:"attempt,omitempty"`
	Outcome   string         `json:"outcome,omitempty"`
	CommitSHA string         `json:"commitSha,omitempty"`
	Source    string         `json:"source,omitempty"`
	Note      string         `json:"note,omitempty"`
	Input     int            `json:"input,omitempty"`
	Output    int            `json:"output,omitempty"`
	Cached    int            `json:"cached,omitempty"`
	Kind      string         `json:"kind,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	// Questions, First, and Auto are the ask_questions op (RFC-114).
	Questions []board.Question `json:"questions,omitempty"`
	First     int              `json:"first,omitempty"`
	Auto      bool             `json:"auto,omitempty"`
	// Handoff is the save_handoff op; HandoffID, OK, and Reason are the
	// handoff_applied op (RFC-118).
	Handoff   *board.Handoff `json:"handoff,omitempty"`
	HandoffID string         `json:"handoffId,omitempty"`
	OK        bool           `json:"ok,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

type Attempt struct {
	ID         string `json:"id"`
	N          int    `json:"n"`
	ParentID   string `json:"parentId"`
	TaskClass  string `json:"taskClass"`
	ModelID    string `json:"modelId"`
	PromptHash string `json:"promptHash"`
	SystemHash string `json:"systemHash"`
	BaseSHA    string `json:"baseSha"`
	MaxTurns   int    `json:"maxTurns"`
}

type Card struct {
	ID             string   `json:"id"`
	ProjectID      string   `json:"projectId"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	LocalCheck     string   `json:"localCheck"`
	RequirementIDs []string `json:"requirementIds"`
	AttemptsUsed   int      `json:"attemptsUsed"`
	MaxAttempts    int      `json:"maxAttempts"`
}

type Convention struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Hash     string `json:"hash"`
	Title    string `json:"title"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Slug     string `json:"slug"`
	UseWhen  string `json:"useWhen"`
	Body     string `json:"body"`
}

// BoardReply carries the result of an op; only the op's fields are set.
type BoardReply struct {
	Card        *Card            `json:"card,omitempty"`
	N           int              `json:"n,omitempty"`
	ParentID    string           `json:"parentId,omitempty"`
	Lines       []string         `json:"lines,omitempty"`
	Text        string           `json:"text,omitempty"`
	Tokens      int              `json:"tokens,omitempty"`
	Conventions []Convention     `json:"conventions,omitempty"`
	Items       []map[string]any `json:"items,omitempty"`
	Questions   []board.Question `json:"questions,omitempty"`
	Handoff     *board.Handoff   `json:"handoff,omitempty"`
}

type ModelCall struct {
	AttemptID string            `json:"attemptId"`
	Turn      int               `json:"turn"`
	Request   providers.Request `json:"request"`
}

type ModelReply struct {
	Response providers.Response `json:"response"`
}

type MCPTools struct {
	Tools   []providers.Tool `json:"tools"`
	Servers []map[string]any `json:"servers"`
}

type MCPCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type MCPResult struct {
	Text    string `json:"text"`
	IsError bool   `json:"isError"`
}

// Report facts (RFC-108).
const (
	FactFinished = "finished"
	FactResume   = "resume"
	FactCheck    = "check"
	FactBlocked  = "blocked"
)

type Report struct {
	Fact      string `json:"fact"`
	AttemptID string `json:"attemptId,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Cause     string `json:"cause,omitempty"`
	Message   string `json:"message,omitempty"`
	Exit      int    `json:"exit"`
	Tail      string `json:"tail,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ReportReply struct {
	Status    string `json:"status"`
	Next      string `json:"next"`
	AttemptID string `json:"attemptId,omitempty"`
	SHA       string `json:"sha,omitempty"`
}
