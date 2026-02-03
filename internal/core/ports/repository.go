package ports

import (
	"context"

	"github.com/diogoX451/archon/internal/core/domain"
)

// NetRepository abstração de persistência
// Implementado em infra (Redis), usado em service
type NetRepository interface {
	Save(ctx context.Context, net *domain.Net) error
	Load(ctx context.Context, netID string) (*domain.Net, error)
	Delete(ctx context.Context, netID string) error

	// Queries para orquestração
	FindNetsWithIdleAgents(ctx context.Context, limit int) ([]string, error)
	FindWaitingAgent(ctx context.Context, correlationID string) (netID string, agentID string, err error)

	// Atualização parcial (otimista)
	UpdateAgent(ctx context.Context, netID string, agent domain.Agent) error
	RegisterWaiting(ctx context.Context, netID, agentID string, need domain.ExternalNeed) error
	ResolveWaiting(ctx context.Context, correlationID string, response domain.Data) error
}
