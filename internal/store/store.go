package store

import (
	"context"
	"time"

	"github.com/diogoX451/archon/pkg/types"
)

type StateStore interface {
	// Workflow lifecycle
	CreateWorkflow(ctx context.Context, state *types.WorkflowState) error
	GetWorkflow(ctx context.Context, workflowID types.WorkflowID) (*types.WorkflowState, error)
	UpdateWorkflow(ctx context.Context, state *types.WorkflowState) error
	DeleteWorkflow(ctx context.Context, workflowID types.WorkflowID) error

	// Agente específico
	GetAgent(ctx context.Context, workflowID types.WorkflowID, agentID types.AgentID) (*types.AgentSnapshot, error)
	UpdateAgent(ctx context.Context, workflowID types.WorkflowID, agent *types.AgentSnapshot) error

	// Queries para o orchestrator
	FindAgentsByState(ctx context.Context, state types.AgentState, limit int) ([]types.WorkUnit, error)
	FindAgentWaitingFor(ctx context.Context, correlationID string) (*types.WorkUnit, error)

	// Controle de espera
	RegisterWaiting(ctx context.Context, workflowID types.WorkflowID, agentID types.AgentID, need *types.ExternalNeed) error
	ResolveWaiting(ctx context.Context, correlationID string, response types.Data) (*types.WorkUnit, error)

	// Listagem
	ListActiveWorkflows(ctx context.Context, userID string) ([]types.WorkflowID, error)

	// Cleanup
	SetTTL(ctx context.Context, workflowID types.WorkflowID, ttl time.Duration) error

	// Transação otimista (para concorrência)
	UpdateWithVersion(ctx context.Context, workflowID types.WorkflowID, fn func(*types.WorkflowState) error) error

	Close() error
}
