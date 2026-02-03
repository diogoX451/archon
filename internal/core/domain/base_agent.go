package domain

import (
	"time"
)

// BaseAgent implementa comportamentos comuns
type BaseAgent struct {
	id            string
	agentType     string
	state         AgentState
	config        Data
	input         Data
	output        Data
	principalPort PortID
	auxPorts      []PortID
	createdAt     time.Time
	updatedAt     time.Time
}

func NewBaseAgent(id, agentType string, config Data) BaseAgent {
	now := time.Now()
	return BaseAgent{
		id:        id,
		agentType: agentType,
		state:     StateIdle,
		config:    config,
		auxPorts:  make([]PortID, 0),
		createdAt: now,
		updatedAt: now,
	}
}

// Getters
func (b *BaseAgent) GetID() string               { return b.id }
func (b *BaseAgent) GetType() string             { return b.agentType }
func (b *BaseAgent) GetState() AgentState        { return b.state }
func (b *BaseAgent) SetState(s AgentState)       { b.state = s; b.updatedAt = time.Now() }
func (b *BaseAgent) GetInput() Data              { return b.input }
func (b *BaseAgent) SetInput(d Data)             { b.input = d }
func (b *BaseAgent) GetOutput() Data             { return b.output }
func (b *BaseAgent) SetOutput(d Data)            { b.output = d; b.updatedAt = time.Now() }
func (b *BaseAgent) GetConfig() Data             { return b.config }
func (b *BaseAgent) GetPrincipalPort() PortID    { return b.principalPort }
func (b *BaseAgent) GetAuxiliaryPorts() []PortID { return b.auxPorts }
func (b *BaseAgent) SetPrincipalPort(port PortID) {
	b.principalPort = port
}
func (b *BaseAgent) SetAuxiliaryPorts(ports []PortID) {
	b.auxPorts = ports
}

func (b *BaseAgent) GetPort(name string) (PortID, bool) {
	if b.principalPort.PortName == name {
		return b.principalPort, true
	}
	for _, p := range b.auxPorts {
		if p.PortName == name {
			return p, true
		}
	}
	return PortID{}, false
}

func (b *BaseAgent) baseSerialize() AgentSnapshot {
	ports := []PortSnapshot{
		{Name: b.principalPort.PortName, Type: "principal"},
	}
	for _, aux := range b.auxPorts {
		ports = append(ports, PortSnapshot{Name: aux.PortName, Type: "auxiliary"})
	}

	return AgentSnapshot{
		ID:        b.id,
		Type:      b.agentType,
		State:     b.state.String(),
		Config:    b.config,
		Input:     b.input,
		Output:    b.output,
		Ports:     ports,
		CreatedAt: b.createdAt,
		UpdatedAt: b.updatedAt,
	}
}

func (b *BaseAgent) BaseDeserialize(snap AgentSnapshot) {
	b.id = snap.ID
	b.agentType = snap.Type
	b.state = parseState(snap.State)
	b.config = snap.Config
	b.input = snap.Input
	b.output = snap.Output
	b.createdAt = snap.CreatedAt
	b.updatedAt = snap.UpdatedAt
}

func (b *BaseAgent) BaseSerialize() AgentSnapshot {
	return b.baseSerialize()
}

func parseState(s string) AgentState {
	switch s {
	case "idle":
		return StateIdle
	case "running":
		return StateRunning
	case "waiting":
		return StateWaiting
	case "completed":
		return StateCompleted
	case "failed":
		return StateFailed
	default:
		return StateIdle
	}
}
