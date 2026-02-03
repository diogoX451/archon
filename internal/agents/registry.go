package agents

import (
	"fmt"

	"github.com/diogoX451/archon/internal/core/domain"
)

type Registry struct {
	factories map[string]domain.AgentFactory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]domain.AgentFactory),
	}
}

func (r *Registry) Register(agentType string, factory domain.AgentFactory) {
	r.factories[agentType] = factory
}

func (r *Registry) Create(snap domain.AgentSnapshot) (domain.Agent, error) {
	factory, ok := r.factories[snap.Type]
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", snap.Type)
	}
	return factory(snap)
}

func RegisterBuiltins(r *Registry) {
	r.Register("calculator", func(snap domain.AgentSnapshot) (domain.Agent, error) {
		return NewCalculatorAgentFromSnapshot(snap), nil
	})

	r.Register("http", func(snap domain.AgentSnapshot) (domain.Agent, error) {
		return NewHTTPAgentFromSnapshot(snap), nil
	})

	r.Register("transform", func(snap domain.AgentSnapshot) (domain.Agent, error) {
		return NewTransformAgentFromSnapshot(snap), nil
	})
}
