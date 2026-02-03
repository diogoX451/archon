package store

import (
	"context"
	"fmt"

	"github.com/diogoX451/archon/internal/core/domain"
	"github.com/diogoX451/archon/internal/core/ports"
	redisstore "github.com/diogoX451/archon/internal/store/redis"
	"github.com/diogoX451/archon/pkg/types"
)

// NetRepositoryImpl adapta nosso Redis store para a interface do Core
type NetRepositoryImpl struct {
	store    *redisstore.RedisStore
	registry domain.AgentRegistry
}

// Verifica interface
var _ ports.NetRepository = (*NetRepositoryImpl)(nil)

func NewNetRepository(store *redisstore.RedisStore, registry domain.AgentRegistry) *NetRepositoryImpl {
	return &NetRepositoryImpl{store: store, registry: registry}
}

func (r *NetRepositoryImpl) Save(ctx context.Context, net *domain.Net) error {
	snapshot, err := net.Snapshot()
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	// Converte para types.WorkflowState
	state := toWorkflowState(snapshot)
	return r.store.UpdateWorkflow(ctx, &state)
}

func (r *NetRepositoryImpl) Load(ctx context.Context, netID string) (*domain.Net, error) {
	state, err := r.store.GetWorkflow(ctx, types.WorkflowID(netID))
	if err != nil {
		return nil, err
	}

	snapshot := toNetSnapshot(*state)
	return domain.RestoreFromSnapshot(snapshot, r.registry)
}

func (r *NetRepositoryImpl) Delete(ctx context.Context, netID string) error {
	return r.store.DeleteWorkflow(ctx, types.WorkflowID(netID))
}

func (r *NetRepositoryImpl) FindNetsWithIdleAgents(ctx context.Context, limit int) ([]string, error) {
	units, err := r.store.FindAgentsByState(ctx, types.StateIdle, limit)
	if err != nil {
		return nil, err
	}

	// Extrai unique net IDs
	seen := make(map[string]bool)
	var result []string
	for _, u := range units {
		if !seen[string(u.WorkflowID)] {
			seen[string(u.WorkflowID)] = true
			result = append(result, string(u.WorkflowID))
		}
	}
	return result, nil
}

func (r *NetRepositoryImpl) FindWaitingAgent(ctx context.Context, correlationID string) (string, string, error) {
	unit, err := r.store.ResolveWaiting(ctx, correlationID, nil)
	if err != nil {
		return "", "", err
	}
	return string(unit.WorkflowID), string(unit.AgentID), nil
}

func (r *NetRepositoryImpl) UpdateAgent(ctx context.Context, netID string, agent domain.Agent) error {
	snapshot, err := agent.Serialize()
	if err != nil {
		return err
	}
	typesSnapshot := toTypesAgentSnapshot(snapshot)
	return r.store.UpdateAgent(ctx, types.WorkflowID(netID), &typesSnapshot)
}

func (r *NetRepositoryImpl) RegisterWaiting(ctx context.Context, netID, agentID string, need domain.ExternalNeed) error {
	typesNeed := toTypesExternalNeed(need)
	return r.store.RegisterWaiting(ctx, types.WorkflowID(netID), types.AgentID(agentID), &typesNeed)
}

func (r *NetRepositoryImpl) ResolveWaiting(ctx context.Context, correlationID string, response domain.Data) error {
	_, err := r.store.ResolveWaiting(ctx, correlationID, response)
	return err
}

// Helpers de conversão
func toWorkflowState(snapshot domain.NetSnapshot) types.WorkflowState {
	state := types.WorkflowState{
		ID:          types.WorkflowID(snapshot.ID),
		UserID:      snapshot.UserID,
		Status:      types.WorkflowRunning,
		CreatedAt:   snapshot.CreatedAt,
		UpdatedAt:   snapshot.UpdatedAt,
		Agents:      make(map[types.AgentID]*types.AgentSnapshot),
		Connections: make([]types.ConnectionSnapshot, 0, len(snapshot.Connections)),
		ActivePairs: make([]types.ActivePairRef, 0, len(snapshot.ActivePairs)),
	}

	for _, agent := range snapshot.Agents {
		typesSnap := toTypesAgentSnapshot(agent)
		state.Agents[typesSnap.ID] = &typesSnap
	}

	for _, conn := range snapshot.Connections {
		state.Connections = append(state.Connections, types.ConnectionSnapshot{
			From:      types.AgentID(conn.From.AgentID),
			FromPort:  conn.From.PortName,
			To:        types.AgentID(conn.To.AgentID),
			ToPort:    conn.To.PortName,
			CreatedAt: snapshot.UpdatedAt,
		})
	}

	for _, pair := range snapshot.ActivePairs {
		state.ActivePairs = append(state.ActivePairs, types.ActivePairRef{
			AgentA:    types.AgentID(pair.AgentAID),
			AgentB:    types.AgentID(pair.AgentBID),
			CreatedAt: snapshot.UpdatedAt,
		})
	}

	return state
}

func toNetSnapshot(state types.WorkflowState) domain.NetSnapshot {
	snapshot := domain.NetSnapshot{
		ID:          string(state.ID),
		UserID:      state.UserID,
		CreatedAt:   state.CreatedAt,
		UpdatedAt:   state.UpdatedAt,
		Agents:      make([]domain.AgentSnapshot, 0, len(state.Agents)),
		Connections: make([]domain.Connection, 0, len(state.Connections)),
		ActivePairs: make([]domain.ActivePair, 0, len(state.ActivePairs)),
	}

	for _, agent := range state.Agents {
		snapshot.Agents = append(snapshot.Agents, toDomainAgentSnapshot(*agent))
	}

	for _, conn := range state.Connections {
		snapshot.Connections = append(snapshot.Connections, domain.Connection{
			From: domain.PortID{AgentID: string(conn.From), PortName: conn.FromPort},
			To:   domain.PortID{AgentID: string(conn.To), PortName: conn.ToPort},
		})
	}

	for _, pair := range state.ActivePairs {
		snapshot.ActivePairs = append(snapshot.ActivePairs, domain.ActivePair{
			AgentAID: string(pair.AgentA),
			AgentBID: string(pair.AgentB),
		})
	}

	return snapshot
}

func toTypesAgentSnapshot(snapshot domain.AgentSnapshot) types.AgentSnapshot {
	state := types.StateIdle
	switch snapshot.State {
	case "idle":
		state = types.StateIdle
	case "running":
		state = types.StateRunning
	case "waiting":
		state = types.StateWaiting
	case "completed":
		state = types.StateCompleted
	case "failed":
		state = types.StateFailed
	}
	return types.AgentSnapshot{
		ID:        types.AgentID(snapshot.ID),
		Type:      snapshot.Type,
		State:     state,
		Config:    snapshot.Config,
		Input:     snapshot.Input,
		Output:    snapshot.Output,
		Need:      toTypesExternalNeedPtr(snapshot.Need),
		UpdatedAt: snapshot.UpdatedAt,
	}
}

func toTypesExternalNeed(need domain.ExternalNeed) types.ExternalNeed {
	return types.ExternalNeed{
		Type:          need.Type,
		CorrelationID: need.CorrelationID,
		Payload:       need.Payload,
		CreatedAt:     need.CreatedAt,
	}
}

func toTypesExternalNeedPtr(need *domain.ExternalNeed) *types.ExternalNeed {
	if need == nil {
		return nil
	}
	mapped := toTypesExternalNeed(*need)
	return &mapped
}

func toDomainAgentSnapshot(snapshot types.AgentSnapshot) domain.AgentSnapshot {
	return domain.AgentSnapshot{
		ID:        string(snapshot.ID),
		Type:      snapshot.Type,
		State:     snapshot.State.String(),
		Config:    snapshot.Config,
		Input:     snapshot.Input,
		Output:    snapshot.Output,
		Need:      toDomainExternalNeedPtr(snapshot.Need),
		CreatedAt: snapshot.UpdatedAt,
		UpdatedAt: snapshot.UpdatedAt,
	}
}

func toDomainExternalNeedPtr(need *types.ExternalNeed) *domain.ExternalNeed {
	if need == nil {
		return nil
	}
	return &domain.ExternalNeed{
		Type:          need.Type,
		CorrelationID: need.CorrelationID,
		Payload:       need.Payload,
		CreatedAt:     need.CreatedAt,
	}
}
