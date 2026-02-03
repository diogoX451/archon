package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diogoX451/archon/internal/core/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TransformAgent transforma JSON usando path expressions
type TransformAgent struct {
	domain.BaseAgent
	Rules []TransformRule
}

type TransformRule struct {
	From string `json:"from"` // path source (gjson)
	To   string `json:"to"`   // path target (sjson)
}

func NewTransformAgent(id string, config domain.Data) *TransformAgent {
	base := domain.NewBaseAgent(id, "transform", config)
	base.SetPrincipalPort(domain.PortID{AgentID: id, PortName: "input"})
	base.SetAuxiliaryPorts([]domain.PortID{
		{AgentID: id, PortName: "output"},
	})

	var cfg struct {
		Rules []TransformRule `json:"rules"`
	}
	json.Unmarshal(config, &cfg)

	return &TransformAgent{
		BaseAgent: base,
		Rules:     cfg.Rules,
	}
}

func NewTransformAgentFromSnapshot(snap domain.AgentSnapshot) *TransformAgent {
	var cfg struct {
		Rules []TransformRule `json:"rules"`
	}
	json.Unmarshal(snap.Config, &cfg)

	agent := &TransformAgent{
		BaseAgent: domain.NewBaseAgent(snap.ID, snap.Type, snap.Config),
		Rules:     cfg.Rules,
	}
	agent.BaseAgent.SetPrincipalPort(domain.PortID{AgentID: snap.ID, PortName: "input"})
	agent.BaseAgent.SetAuxiliaryPorts([]domain.PortID{
		{AgentID: snap.ID, PortName: "output"},
	})
	agent.BaseAgent.BaseDeserialize(snap)
	return agent
}

func (t *TransformAgent) Execute(ctx context.Context) domain.ExecuteResult {
	input := string(t.GetInput())
	if input == "" {
		input = "{}"
	}

	result := "{}"

	for _, rule := range t.Rules {
		// Extrai valor
		value := gjson.Get(input, rule.From)
		if !value.Exists() {
			continue
		}

		// Seta no resultado
		var err error
		result, err = sjson.Set(result, rule.To, value.Value())
		if err != nil {
			return domain.ExecuteResult{
				State: domain.StateFailed,
				Error: fmt.Errorf("transform failed: %w", err),
			}
		}
	}

	return domain.ExecuteResult{
		State:  domain.StateCompleted,
		Output: []byte(result),
	}
}

func (t *TransformAgent) Resume(ctx context.Context, response domain.Data) domain.ExecuteResult {
	return domain.ExecuteResult{
		State: domain.StateFailed,
		Error: fmt.Errorf("transform does not support resume"),
	}
}

func (t *TransformAgent) Serialize() (domain.AgentSnapshot, error) {
	return t.BaseAgent.BaseSerialize(), nil
}

func (t *TransformAgent) Interact(ctx context.Context, other domain.Agent) domain.InteractionResult {
	exec := t.Execute(ctx)
	result := domain.InteractionResult{
		AgentAState: exec.State,
		Error:       exec.Error,
	}
	if exec.State == domain.StateCompleted {
		result.OutputA = exec.Output
		result.IsFinal = true
		result.Output = exec.Output
	}
	return result
}
