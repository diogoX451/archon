package types

import (
	"encoding/json"
	"time"
)

type WorkflowID string
type AgentID string
type Data = json.RawMessage
type AgentState int

const (
	StateIdle AgentState = iota
	StateRunning
	StateWaiting
	StateCompleted
	StateFailed
)

func (s AgentState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StateWaiting:
		return "waiting"
	case StateCompleted:
		return "completed"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

type ExternalNeed struct {
	Type          string    `json:"type"`
	CorrelationID string    `json:"correlation_id"`
	Payload       Data      `json:"payload"`
	CreatedAt     time.Time `json:"created_at"`
}

type ExecuteResult struct {
	State  AgentState    `json:"state"`
	Need   *ExternalNeed `json:"need,omitempty"`
	Output Data          `json:"output,omitempty"`
	Error  string        `json:"error,omitempty"`
}

type SpawnEvent struct {
	WorkflowID string    `json:"workflow_id"`
	Blueprint  Blueprint `json:"blueprint"`
	Input      Data      `json:"input"`
	CreatedAt  time.Time `json:"created_at"`
}

type Blueprint struct {
	Agents      []AgentDef      `json:"agents"`
	Connections []ConnectionDef `json:"connections"`
}

type AgentDef struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config,omitempty"`
}

type ConnectionDef struct {
	From string `json:"from"`
	To   string `json:"to"`
	As   string `json:"as,omitempty"`
}

type ResponseEvent struct {
	CorrelationID string    `json:"correlation_id"`
	Payload       Data      `json:"payload"`
	ReceivedAt    time.Time `json:"received_at"`
}

type ResultEvent struct {
	WorkflowID string    `json:"workflow_id"`
	Status     string    `json:"status"`
	Output     Data      `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}
