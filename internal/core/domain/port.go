package domain

import (
	"fmt"
)

// PortType discrimina portas
type PortType int

const (
	PortPrincipal PortType = iota
	PortAuxiliary
)

func (p PortType) String() string {
	switch p {
	case PortPrincipal:
		return "principal"
	case PortAuxiliary:
		return "auxiliary"
	default:
		return "unknown"
	}
}

// PortID identifica unicamente uma porta no grafo
type PortID struct {
	AgentID  string
	PortName string
}

func (p PortID) String() string {
	return fmt.Sprintf("%s:%s", p.AgentID, p.PortName)
}

// Connection liga duas portas
type Connection struct {
	From PortID
	To   PortID
}

// PortSnapshot para serialização
type PortSnapshot struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Connected *PortID `json:"connected,omitempty"`
	Data      Data    `json:"data,omitempty"` // Dados "no fio"
}
