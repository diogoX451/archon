package service

import (
	"context"
	"fmt"
	"log"

	"github.com/diogoX451/archon/internal/core/domain"
	"github.com/diogoX451/archon/internal/core/ports"
)

type Executor struct {
	repo     ports.NetRepository
	eventBus ports.EventBus
	registry domain.AgentRegistry
	rules    *domain.RuleRegistry
}

func NewExecutor(repo ports.NetRepository, bus ports.EventBus, registry domain.AgentRegistry, rules *domain.RuleRegistry) *Executor {
	return &Executor{
		repo:     repo,
		eventBus: bus,
		registry: registry,
		rules:    rules,
	}
}

func (e *Executor) FindWaitingAgent(ctx context.Context, correlationID string) (string, string, error) {
	return e.repo.FindWaitingAgent(ctx, correlationID)
}

// ExecuteInteraction processa um par ativo completo
func (e *Executor) ExecuteInteraction(ctx context.Context, netID string, pair domain.ActivePair) error {
	// 1. Carrega net
	net, err := e.repo.Load(ctx, netID)
	if err != nil {
		return fmt.Errorf("load net: %w", err)
	}

	// 2. Executa interação (core puro!)
	result, err := net.ReducePair(ctx, pair, e.rules, e.registry)
	if err != nil {
		return fmt.Errorf("interact: %w", err)
	}

	// 3. Aplica reescrita na estrutura
	if err := e.applyRewrite(net, pair, result); err != nil {
		return fmt.Errorf("apply rewrite: %w", err)
	}

	// 4. Persiste
	if err := e.repo.Save(ctx, net); err != nil {
		return fmt.Errorf("save net: %w", err)
	}

	// 5. Publica eventos de follow-up
	for _, newPair := range result.NewActivePairs {
		if err := e.eventBus.PublishInteraction(ctx, netID, newPair); err != nil {
			log.Printf("failed to publish interaction: %v", err)
		}
	}

	if result.IsFinal {
		if err := e.eventBus.PublishResult(ctx, netID, result.Output); err != nil {
			log.Printf("failed to publish result: %v", err)
		}
	}

	return nil
}

// ResumeAgent acorda agente que estava esperando
func (e *Executor) ResumeAgent(ctx context.Context, netID, agentID string, response domain.Data) error {
	net, err := e.repo.Load(ctx, netID)
	if err != nil {
		return err
	}

	agent, ok := net.GetAgent(agentID)
	if !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	if agent.GetState() != domain.StateWaiting {
		return fmt.Errorf("agent not waiting: %s", agent.GetState())
	}

	// Executa resume (algoritmo puro)
	result := agent.Resume(ctx, response)
	agent.SetState(result.State)

	if result.State == domain.StateCompleted {
		agent.SetOutput(result.Output)
		// Verifica se forma novos pares ativos...
	}

	// Persiste e publica
	if err := e.repo.Save(ctx, net); err != nil {
		return err
	}

	return nil
}

func (e *Executor) applyRewrite(net *domain.Net, executedPair domain.ActivePair, result *domain.InteractionResult) error {
	// Remove agentes antigos
	for _, id := range result.Remove {
		net.RemoveAgent(id)
	}

	// Adiciona novos
	for _, agent := range result.Spawn {
		if err := net.AddAgent(agent); err != nil {
			return err
		}
	}

	// Estabelece conexões (pode formar novos pares ativos)
	for _, conn := range result.NewConnections {
		if pair, ok, err := net.Connect(conn.From, conn.To); err != nil {
			return err
		} else if ok {
			result.NewActivePairs = append(result.NewActivePairs, pair)
		}
	}

	return nil
}
