package types

import (
	"encoding/json"
	"time"
)

type WorkflowState struct {
	ID        WorkflowID     `json:"id"`
	UserID    string         `json:"user_id"`
	Status    WorkflowStatus `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	// Componentes do grafo
	Agents      map[AgentID]*AgentSnapshot `json:"agents"`
	Connections []ConnectionSnapshot       `json:"connections"`

	// Controle de execução
	ActivePairs []ActivePairRef    `json:"active_pairs"`
	Waiting     map[string]AgentID `json:"waiting"` // correlation_id -> agent_id
}

type WorkflowStatus string

const (
	WorkflowRunning   WorkflowStatus = "running"
	WorkflowWaiting   WorkflowStatus = "waiting"
	WorkflowCompleted WorkflowStatus = "completed"
	WorkflowFailed    WorkflowStatus = "failed"
)

type AgentSnapshot struct {
	ID        AgentID         `json:"id"`
	Type      string          `json:"type"`
	State     AgentState      `json:"state"`
	Config    json.RawMessage `json:"config"`
	Input     Data            `json:"input,omitempty"`
	Output    Data            `json:"output,omitempty"`
	Need      *ExternalNeed   `json:"need,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type ConnectionSnapshot struct {
	From      AgentID   `json:"from"`
	FromPort  string    `json:"from_port"`
	To        AgentID   `json:"to"`
	ToPort    string    `json:"to_port"`
	CreatedAt time.Time `json:"created_at"`
}

type ActivePairRef struct {
	AgentA    AgentID   `json:"agent_a"`
	AgentB    AgentID   `json:"agent_b"`
	CreatedAt time.Time `json:"created_at"`
}

type WorkUnit struct {
	WorkflowID WorkflowID    `json:"workflow_id"`
	UserID     string        `json:"user_id"`
	AgentID    AgentID       `json:"agent_id"`
	Operation  Operation     `json:"operation"`
	Input      Data          `json:"input"`
	Need       *ExternalNeed `json:"need,omitempty"`
}

type Operation string

const (
	OpExecute Operation = "execute"
	OpResume  Operation = "resume"
)
