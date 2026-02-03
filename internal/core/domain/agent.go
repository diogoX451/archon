package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Data é opaco (bytes)
type Data = json.RawMessage

// AgentState do ciclo de vida
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

// ExternalNeed = pedido ao mundo externo
type ExternalNeed struct {
	Type          string    `json:"type"`
	CorrelationID string    `json:"correlation_id"`
	Payload       Data      `json:"payload"`
	CreatedAt     time.Time `json:"created_at"`
}

// ExecuteResult resultado da execução
type ExecuteResult struct {
	State  AgentState
	Need   *ExternalNeed // Se StateWaiting
	Output Data          // Se StateCompleted
	Error  error         // Se StateFailed
}

// Agent é a interface que todo nó implementa
// Pura, sem dependências de infraestrutura
type Agent interface {
	// Identidade
	GetID() string
	GetType() string

	// Portas
	GetPrincipalPort() PortID
	GetAuxiliaryPorts() []PortID
	GetPort(name string) (PortID, bool)

	// Estado
	GetState() AgentState
	SetState(state AgentState)

	// Dados
	GetInput() Data
	SetInput(input Data)
	GetOutput() Data
	SetOutput(output Data)

	// Configuração (imutável após criação)
	GetConfig() Data

	// Execução (o coração do algoritmo)
	Execute(ctx context.Context) ExecuteResult
	Resume(ctx context.Context, response Data) ExecuteResult
	Interact(ctx context.Context, other Agent) InteractionResult

	// Serialização (para persistência)
	Serialize() (AgentSnapshot, error)
}

// AgentSnapshot para persistência
type AgentSnapshot struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	State     string         `json:"state"`
	Config    Data           `json:"config"`
	Input     Data           `json:"input,omitempty"`
	Output    Data           `json:"output,omitempty"`
	Need      *ExternalNeed  `json:"need,omitempty"`
	Ports     []PortSnapshot `json:"ports"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// AgentFactory cria agentes de snapshots
type AgentFactory func(snapshot AgentSnapshot) (Agent, error)

// Registry de factories (injetado no core)
type AgentRegistry interface {
	Register(agentType string, factory AgentFactory)
	Create(snapshot AgentSnapshot) (Agent, error)
}
