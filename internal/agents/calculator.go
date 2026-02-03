package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diogoX451/archon/internal/core/domain"
)

// CalculatorAgent executa operações matemáticas sem I/O externo
type CalculatorAgent struct {
	domain.BaseAgent
	Operation string
}

// NewCalculatorAgent cria novo agente
func NewCalculatorAgent(id string, config domain.Data) *CalculatorAgent {
	base := domain.NewBaseAgent(id, "calculator", config)
	base.SetPrincipalPort(domain.PortID{AgentID: id, PortName: "input"})
	base.SetAuxiliaryPorts([]domain.PortID{
		{AgentID: id, PortName: "output"},
	})

	var cfg struct {
		Operation string `json:"operation"`
	}
	json.Unmarshal(config, &cfg)

	return &CalculatorAgent{
		BaseAgent: base,
		Operation: cfg.Operation,
	}
}

// NewCalculatorAgentFromSnapshot restaura de snapshot
func NewCalculatorAgentFromSnapshot(snap domain.AgentSnapshot) *CalculatorAgent {
	var cfg struct {
		Operation string `json:"operation"`
	}
	json.Unmarshal(snap.Config, &cfg)

	agent := &CalculatorAgent{
		BaseAgent: domain.NewBaseAgent(snap.ID, snap.Type, snap.Config),
		Operation: cfg.Operation,
	}
	agent.BaseAgent.SetPrincipalPort(domain.PortID{AgentID: snap.ID, PortName: "input"})
	agent.BaseAgent.SetAuxiliaryPorts([]domain.PortID{
		{AgentID: snap.ID, PortName: "output"},
	})
	agent.BaseAgent.BaseDeserialize(snap)
	return agent
}

func (c *CalculatorAgent) Execute(ctx context.Context) domain.ExecuteResult {
	// Parse input: array de números
	var numbers []float64
	if err := json.Unmarshal(c.GetInput(), &numbers); err != nil {
		return domain.ExecuteResult{
			State: domain.StateFailed,
			Error: fmt.Errorf("invalid input: %w", err),
		}
	}

	// Computa (CPU puro, nunca espera)
	var result float64
	switch c.Operation {
	case "sum":
		for _, n := range numbers {
			result += n
		}
	case "multiply":
		result = 1
		for _, n := range numbers {
			result *= n
		}
	case "average":
		if len(numbers) > 0 {
			sum := 0.0
			for _, n := range numbers {
				sum += n
			}
			result = sum / float64(len(numbers))
		}
	default:
		return domain.ExecuteResult{
			State: domain.StateFailed,
			Error: fmt.Errorf("unknown operation: %s", c.Operation),
		}
	}

	// Prepara output
	output, _ := json.Marshal(map[string]float64{"result": result})

	return domain.ExecuteResult{
		State:  domain.StateCompleted,
		Output: output,
	}
}

func (c *CalculatorAgent) Resume(ctx context.Context, response domain.Data) domain.ExecuteResult {
	// Calculator nunca espera, Resume não deve ser chamado
	return domain.ExecuteResult{
		State: domain.StateFailed,
		Error: fmt.Errorf("calculator does not support resume"),
	}
}

func (c *CalculatorAgent) Serialize() (domain.AgentSnapshot, error) {
	snap := c.BaseAgent.BaseSerialize()
	return snap, nil
}

func (c *CalculatorAgent) Interact(ctx context.Context, other domain.Agent) domain.InteractionResult {
	exec := c.Execute(ctx)
	result := domain.InteractionResult{
		AgentAState: exec.State,
		Remove:      []string{},
		Spawn:       []domain.Agent{},
		Error:       exec.Error,
	}
	if exec.State == domain.StateCompleted {
		result.OutputA = exec.Output
		result.IsFinal = true
		result.Output = exec.Output
	}
	return result
}
