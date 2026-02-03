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

	// 2. Captura input de fallback a partir do par (se existir)
	var pairInput domain.Data
	if a, ok := net.GetAgent(pair.AgentAID); ok {
		if a.GetOutput() != nil {
			pairInput = a.GetOutput()
		} else if a.GetInput() != nil {
			pairInput = a.GetInput()
		}
	}
	if pairInput == nil {
		if b, ok := net.GetAgent(pair.AgentBID); ok {
			if b.GetOutput() != nil {
				pairInput = b.GetOutput()
			} else if b.GetInput() != nil {
				pairInput = b.GetInput()
			}
		}
	}

	// 3. Executa interação (core puro!)
	result, err := net.ReducePair(ctx, pair, e.rules, e.registry)
	if err != nil {
		return fmt.Errorf("interact: %w", err)
	}

	// 4. Aplica reescrita na estrutura
	if err := e.applyRewrite(net, pair, result); err != nil {
		return fmt.Errorf("apply rewrite: %w", err)
	}

	// 5. Publica needs para agentes recém-criados (ex.: http)
	if err := e.publishNeedsForSpawn(ctx, net, result.Spawn, pairInput); err != nil {
		return fmt.Errorf("publish needs: %w", err)
	}

	// 6. Persiste
	if err := e.repo.Save(ctx, net); err != nil {
		return fmt.Errorf("save net: %w", err)
	}

	// 7. Publica eventos de follow-up
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

func (e *Executor) publishNeedsForSpawn(ctx context.Context, net *domain.Net, spawned []domain.Agent, fallbackInput domain.Data) error {
	for _, agent := range spawned {
		if agent.GetType() != "transform" {
			continue
		}
		if agent.GetOutput() == nil && agent.GetInput() != nil {
			execUp := agent.Execute(ctx)
			if execUp.State == domain.StateCompleted {
				agent.SetOutput(execUp.Output)
			}
		}
		if fallbackInput == nil {
			if out := agent.GetOutput(); out != nil {
				fallbackInput = out
			} else if in := agent.GetInput(); in != nil {
				fallbackInput = in
			}
		}
	}

	for _, agent := range spawned {
		if agent.GetType() != "http" {
			continue
		}
		log.Printf("http agent pre-exec net=%s agent=%s input_bytes=%d", net.ID, agent.GetID(), len(agent.GetInput()))
		if agent.GetInput() == nil {
			if trigger, ok := agent.GetPort("trigger"); ok {
				if upstreamPort, ok := net.GetConnection(trigger); ok {
					if upstream, ok := net.GetAgent(upstreamPort.AgentID); ok && upstream.GetType() == "transform" {
						log.Printf("http agent upstream net=%s agent=%s upstream=%s input_bytes=%d output_bytes=%d",
							net.ID, agent.GetID(), upstream.GetID(), len(upstream.GetInput()), len(upstream.GetOutput()))
						if upstream.GetOutput() == nil && upstream.GetInput() != nil {
							execUp := upstream.Execute(ctx)
							if execUp.State == domain.StateCompleted {
								upstream.SetOutput(execUp.Output)
							}
						}
						if out := upstream.GetOutput(); out != nil {
							agent.SetInput(out)
						} else if in := upstream.GetInput(); in != nil {
							agent.SetInput(in)
						}
					}
				}
			}
		}
		if agent.GetInput() == nil && fallbackInput != nil {
			agent.SetInput(fallbackInput)
		}
		exec := agent.Execute(ctx)
		if exec.Error != nil {
			log.Printf("http execute error net=%s agent=%s err=%v", net.ID, agent.GetID(), exec.Error)
		}
		agent.SetState(exec.State)
		if exec.State == domain.StateCompleted {
			agent.SetOutput(exec.Output)
		}
		if exec.State == domain.StateWaiting && exec.Need != nil {
			log.Printf("publishing need net=%s agent=%s type=%s", net.ID, agent.GetID(), exec.Need.Type)
			if err := e.repo.RegisterWaiting(ctx, net.ID, agent.GetID(), *exec.Need); err != nil {
				return err
			}
			if err := e.eventBus.PublishNeed(ctx, net.ID, *exec.Need); err != nil {
				return err
			}
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
