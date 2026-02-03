package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/diogoX451/archon/internal/store"
	"github.com/diogoX451/archon/pkg/types"
	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

var _ store.StateStore = (*RedisStore)(nil)

type Config struct {
	Addr       string
	Password   string
	DB         int
	PoolSize   int
	DefaultTTL time.Duration
}

func New(cfg Config) (*RedisStore, error) {
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = 24 * time.Hour
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisStore{
		client: client,
		ttl:    cfg.DefaultTTL,
	}, nil
}

// Chaves Redis:
// workflow:{id} -> hash com dados do workflow
// workflow:{id}:agents:{agent_id} -> hash com snapshot do agente
// waiting:{correlation_id} -> string com workflow_id:agent_id
// index:state:{state} -> set de workflow_id:agent_id (para queries)
// user:{user_id}:workflows -> set de workflow_ids
// rule:{a}|{b} -> json RuleDef
// rules:index -> set de rule keys

func (r *RedisStore) workflowKey(id types.WorkflowID) string {
	return fmt.Sprintf("workflow:%s", id)
}

func (r *RedisStore) agentKey(workflowID types.WorkflowID, agentID types.AgentID) string {
	return fmt.Sprintf("workflow:%s:agent:%s", workflowID, agentID)
}

func (r *RedisStore) waitingKey(correlationID string) string {
	return fmt.Sprintf("waiting:%s", correlationID)
}

func (r *RedisStore) stateIndexKey(state types.AgentState) string {
	return fmt.Sprintf("index:state:%s", state)
}

func (r *RedisStore) userIndexKey(userID string) string {
	return fmt.Sprintf("user:%s:workflows", userID)
}

func (r *RedisStore) ruleKey(aType, bType string) string {
	typesKey := []string{aType, bType}
	sort.Strings(typesKey)
	return fmt.Sprintf("rule:%s|%s", typesKey[0], typesKey[1])
}

func (r *RedisStore) rulesIndexKey() string {
	return "rules:index"
}

func (r *RedisStore) CreateWorkflow(ctx context.Context, state *types.WorkflowState) error {
	pipe := r.client.Pipeline()

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	key := r.workflowKey(state.ID)
	pipe.Set(ctx, key, data, 0)

	pipe.SAdd(ctx, r.userIndexKey(state.UserID), string(state.ID))

	for agentID, agent := range state.Agents {
		agentData, _ := json.Marshal(agent)
		pipe.Set(ctx, r.agentKey(state.ID, agentID), agentData, r.ttl)

		if agent.State == types.StateIdle || agent.State == types.StateWaiting {
			pipe.SAdd(ctx, r.stateIndexKey(agent.State), fmt.Sprintf("%s:%s", state.ID, agentID))
		}
	}

	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisStore) GetWorkflow(ctx context.Context, workflowID types.WorkflowID) (*types.WorkflowState, error) {
	key := r.workflowKey(workflowID)

	data, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	var state types.WorkflowState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	// Reconstrói mapa de agents do índice
	pattern := fmt.Sprintf("workflow:%s:agent:*", workflowID)
	iter := r.client.Scan(ctx, 0, pattern, 100).Iterator()

	state.Agents = make(map[types.AgentID]*types.AgentSnapshot)
	for iter.Next(ctx) {
		agentData, err := r.client.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}

		var agent types.AgentSnapshot
		if err := json.Unmarshal([]byte(agentData), &agent); err != nil {
			continue
		}
		state.Agents[agent.ID] = &agent
	}

	return &state, nil
}

func (r *RedisStore) UpdateWorkflow(ctx context.Context, state *types.WorkflowState) error {
	pipe := r.client.Pipeline()

	state.UpdatedAt = time.Now()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	key := r.workflowKey(state.ID)
	pipe.Set(ctx, key, data, r.ttl)

	for _, agent := range state.Agents {
		agentData, _ := json.Marshal(agent)
		pipe.Set(ctx, r.agentKey(state.ID, agent.ID), agentData, r.ttl)

		member := fmt.Sprintf("%s:%s", state.ID, agent.ID)
		pipe.SRem(ctx, r.stateIndexKey(types.StateIdle), member)
		pipe.SRem(ctx, r.stateIndexKey(types.StateWaiting), member)
		pipe.SRem(ctx, r.stateIndexKey(types.StateRunning), member)

		if agent.State == types.StateIdle || agent.State == types.StateWaiting || agent.State == types.StateRunning {
			pipe.SAdd(ctx, r.stateIndexKey(agent.State), member)
		}
	}

	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisStore) UpdateAgent(ctx context.Context, workflowID types.WorkflowID, agent *types.AgentSnapshot) error {
	pipe := r.client.Pipeline()

	agent.UpdatedAt = time.Now()
	data, err := json.Marshal(agent)
	if err != nil {
		return err
	}

	agentKey := r.agentKey(workflowID, agent.ID)
	pipe.Set(ctx, agentKey, data, r.ttl)

	member := fmt.Sprintf("%s:%s", workflowID, agent.ID)

	pipe.SRem(ctx, r.stateIndexKey(types.StateIdle), member)
	pipe.SRem(ctx, r.stateIndexKey(types.StateWaiting), member)
	pipe.SRem(ctx, r.stateIndexKey(types.StateRunning), member)
	pipe.SRem(ctx, r.stateIndexKey(types.StateCompleted), member)
	pipe.SRem(ctx, r.stateIndexKey(types.StateFailed), member)

	if agent.State == types.StateIdle || agent.State == types.StateWaiting {
		pipe.SAdd(ctx, r.stateIndexKey(agent.State), member)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}

	// Refresh workflow updated_at safely (workflow is stored as JSON string, not a hash).
	workflowKey := r.workflowKey(workflowID)
	workflowData, err := r.client.Get(ctx, workflowKey).Result()
	if err != nil {
		return nil
	}

	var workflow types.WorkflowState
	if err := json.Unmarshal([]byte(workflowData), &workflow); err != nil {
		return nil
	}

	workflow.UpdatedAt = time.Now()
	updatedData, err := json.Marshal(&workflow)
	if err != nil {
		return nil
	}

	return r.client.Set(ctx, workflowKey, updatedData, r.ttl).Err()
}

func (r *RedisStore) FindAgentsByState(ctx context.Context, state types.AgentState, limit int) ([]types.WorkUnit, error) {
	indexKey := r.stateIndexKey(state)

	// Pega membros do set
	members, err := r.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		return nil, err
	}

	var units []types.WorkUnit
	for i, member := range members {
		if i >= limit {
			break
		}

		// Parse workflow_id:agent_id
		workflowID, agentID, ok := splitWorkflowAgent(member)
		if !ok {
			continue
		}

		// Carrega agente para pegar input
		agentData, err := r.client.Get(ctx, r.agentKey(types.WorkflowID(workflowID), types.AgentID(agentID))).Result()
		if err != nil {
			continue
		}

		var agent types.AgentSnapshot
		if err := json.Unmarshal([]byte(agentData), &agent); err != nil {
			continue
		}

		// Carrega workflow para user_id
		workflowData, err := r.client.Get(ctx, r.workflowKey(types.WorkflowID(workflowID))).Result()
		if err != nil {
			continue
		}

		var workflow types.WorkflowState
		if err := json.Unmarshal([]byte(workflowData), &workflow); err != nil {
			continue
		}

		unit := types.WorkUnit{
			WorkflowID: types.WorkflowID(workflowID),
			UserID:     workflow.UserID,
			AgentID:    types.AgentID(agentID),
			Operation:  types.OpExecute,
			Input:      agent.Input,
		}

		if state == types.StateWaiting && agent.Need != nil {
			unit.Operation = types.OpResume
			unit.Need = agent.Need
		}

		units = append(units, unit)
	}

	return units, nil
}

// RegisterWaiting marca agente como esperando resposta externa
func (r *RedisStore) RegisterWaiting(ctx context.Context, workflowID types.WorkflowID, agentID types.AgentID, need *types.ExternalNeed) error {
	pipe := r.client.Pipeline()

	// Mapeia correlation_id -> workflow:agent
	mapping := fmt.Sprintf("%s:%s", workflowID, agentID)
	pipe.Set(ctx, r.waitingKey(need.CorrelationID), mapping, r.ttl)

	// Move de idle para waiting no índice
	member := fmt.Sprintf("%s:%s", workflowID, agentID)
	pipe.SRem(ctx, r.stateIndexKey(types.StateIdle), member)
	pipe.SAdd(ctx, r.stateIndexKey(types.StateWaiting), member)

	_, err := pipe.Exec(ctx)
	return err
}

// ResolveWaiting encontra e prepara agente para resumir
func (r *RedisStore) ResolveWaiting(ctx context.Context, correlationID string, response types.Data) (*types.WorkUnit, error) {
	// Encontra qual agente estava esperando
	waitingKey := r.waitingKey(correlationID)
	mapping, err := r.client.Get(ctx, waitingKey).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("no agent waiting for correlation: %s", correlationID)
	}
	if err != nil {
		return nil, err
	}

	// Parse workflow_id:agent_id
	workflowID, agentID, ok := splitWorkflowAgent(mapping)
	if !ok {
		return nil, fmt.Errorf("invalid waiting mapping: %s", mapping)
	}

	// Remove do índice de waiting
	member := fmt.Sprintf("%s:%s", workflowID, agentID)
	r.client.SRem(ctx, r.stateIndexKey(types.StateWaiting), member)

	// Remove a chave de waiting
	r.client.Del(ctx, waitingKey)

	// Carrega workflow para user_id
	workflow, err := r.GetWorkflow(ctx, types.WorkflowID(workflowID))
	if err != nil {
		return nil, err
	}

	// Atualiza agente com resposta (guarda temporariamente)
	agent := workflow.Agents[types.AgentID(agentID)]
	agent.State = types.StateIdle // Vai para idle, mas com input especial
	// O input real vem no Resume via response

	if err := r.UpdateAgent(ctx, types.WorkflowID(workflowID), agent); err != nil {
		return nil, err
	}

	return &types.WorkUnit{
		WorkflowID: types.WorkflowID(workflowID),
		UserID:     workflow.UserID,
		AgentID:    types.AgentID(agentID),
		Operation:  types.OpResume,
		Input:      response, // A resposta externa vira input do resume
	}, nil
}

func splitWorkflowAgent(value string) (string, string, bool) {
	workflowID, agentID, ok := strings.Cut(value, ":")
	if !ok || workflowID == "" || agentID == "" {
		return "", "", false
	}
	return workflowID, agentID, true
}

// DeleteWorkflow remove tudo
func (r *RedisStore) DeleteWorkflow(ctx context.Context, workflowID types.WorkflowID) error {
	pipe := r.client.Pipeline()

	// Pega workflow para saber user_id
	workflow, err := r.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}

	// Remove índice do usuário
	pipe.SRem(ctx, r.userIndexKey(workflow.UserID), string(workflowID))

	// Remove agentes
	for agentID := range workflow.Agents {
		pipe.Del(ctx, r.agentKey(workflowID, agentID))
		member := fmt.Sprintf("%s:%s", workflowID, agentID)
		pipe.SRem(ctx, r.stateIndexKey(types.StateIdle), member)
		pipe.SRem(ctx, r.stateIndexKey(types.StateWaiting), member)
	}

	// Remove workflow
	pipe.Del(ctx, r.workflowKey(workflowID))

	_, err = pipe.Exec(ctx)
	return err
}

// SetTTL ajusta tempo de vida
func (r *RedisStore) SetTTL(ctx context.Context, workflowID types.WorkflowID, ttl time.Duration) error {
	// Renova TTL em todas as chaves relacionadas
	pattern := fmt.Sprintf("workflow:%s*", workflowID)
	iter := r.client.Scan(ctx, 0, pattern, 100).Iterator()

	for iter.Next(ctx) {
		r.client.Expire(ctx, iter.Val(), ttl)
	}

	return iter.Err()
}

// Close fecha conexão
func (r *RedisStore) Close() error {
	return r.client.Close()
}

// Métodos não implementados (para interface)
func (r *RedisStore) GetAgent(ctx context.Context, workflowID types.WorkflowID, agentID types.AgentID) (*types.AgentSnapshot, error) {
	data, err := r.client.Get(ctx, r.agentKey(workflowID, agentID)).Result()
	if err != nil {
		return nil, err
	}

	var agent types.AgentSnapshot
	if err := json.Unmarshal([]byte(data), &agent); err != nil {
		return nil, err
	}

	return &agent, nil
}

func (r *RedisStore) FindAgentWaitingFor(ctx context.Context, correlationID string) (*types.WorkUnit, error) {
	return r.ResolveWaiting(ctx, correlationID, nil)
}

func (r *RedisStore) ListActiveWorkflows(ctx context.Context, userID string) ([]types.WorkflowID, error) {
	ids, err := r.client.SMembers(ctx, r.userIndexKey(userID)).Result()
	if err != nil {
		return nil, err
	}

	result := make([]types.WorkflowID, len(ids))
	for i, id := range ids {
		result[i] = types.WorkflowID(id)
	}

	return result, nil
}

func (r *RedisStore) UpdateWithVersion(ctx context.Context, workflowID types.WorkflowID, fn func(*types.WorkflowState) error) error {
	// Simplificado: lê, modifica, salva (optimistic locking seria com WATCH)
	state, err := r.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}

	if err := fn(state); err != nil {
		return err
	}

	return r.UpdateWorkflow(ctx, state)
}

func (r *RedisStore) SaveRule(ctx context.Context, rule types.RuleDef) error {
	if rule.AgentAType == "" || rule.AgentBType == "" {
		return fmt.Errorf("rule types required")
	}

	key := r.ruleKey(rule.AgentAType, rule.AgentBType)
	data, err := json.Marshal(rule)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, r.ttl)
	pipe.SAdd(ctx, r.rulesIndexKey(), key)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisStore) GetRule(ctx context.Context, aType, bType string) (*types.RuleDef, error) {
	key := r.ruleKey(aType, bType)
	data, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("rule not found for %s,%s", aType, bType)
	}
	if err != nil {
		return nil, err
	}

	var rule types.RuleDef
	if err := json.Unmarshal([]byte(data), &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *RedisStore) ListRules(ctx context.Context) ([]types.RuleDef, error) {
	keys, err := r.client.SMembers(ctx, r.rulesIndexKey()).Result()
	if err != nil {
		return nil, err
	}

	result := make([]types.RuleDef, 0, len(keys))
	for _, key := range keys {
		data, err := r.client.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var rule types.RuleDef
		if err := json.Unmarshal([]byte(data), &rule); err != nil {
			continue
		}
		result = append(result, rule)
	}

	return result, nil
}
