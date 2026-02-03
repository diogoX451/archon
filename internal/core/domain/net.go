package domain

import (
	"context"
	"fmt"
	"time"
)

// Net é o grafo de agentes
// Entidade de agregado (Aggregate Root)
type Net struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	UpdatedAt time.Time

	agents      map[string]Agent
	connections map[PortID]*PortID // mapeia porta -> porta conectada
	activePairs []ActivePair       // pares prontos para executar
}

// ActivePair identifica dois agentes que podem interagir
type ActivePair struct {
	AgentAID string
	AgentBID string
	PortA    PortID
	PortB    PortID
}

// InteractionResult resultado de uma interação
type InteractionResult struct {
	// Estado resultante do agente A após a interação
	AgentAState AgentState

	// Saída gerada pelo agente A (quando completa)
	OutputA Data

	// Agentes a remover (se reescrita os substituiu)
	Remove []string

	// Novos agentes a adicionar
	Spawn []Agent

	// Novas conexões a estabelecer
	NewConnections []Connection

	// Novos pares ativos formados pelas novas conexões
	NewActivePairs []ActivePair

	// Se a interação produziu output final
	IsFinal bool
	Output  Data

	// Erro da interação (se falhou)
	Error error
}

// NetSnapshot para persistência
type NetSnapshot struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Agents      []AgentSnapshot `json:"agents"`
	Connections []Connection    `json:"connections"`
	ActivePairs []ActivePair    `json:"active_pairs"`
}

// --- Métodos do Net ---

func NewNet(id, userID string) *Net {
	return &Net{
		ID:          id,
		UserID:      userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		agents:      make(map[string]Agent),
		connections: make(map[PortID]*PortID),
		activePairs: []ActivePair{},
	}
}

func (n *Net) AddAgent(agent Agent) error {
	if _, exists := n.agents[agent.GetID()]; exists {
		return fmt.Errorf("agent %s already exists", agent.GetID())
	}
	n.agents[agent.GetID()] = agent
	n.UpdatedAt = time.Now()
	return nil
}

func (n *Net) GetAgent(id string) (Agent, bool) {
	agent, ok := n.agents[id]
	return agent, ok
}

func (n *Net) GetConnection(port PortID) (*PortID, bool) {
	conn := n.connections[port]
	if conn == nil {
		return nil, false
	}
	c := *conn
	return &c, true
}

func (n *Net) RemoveAgent(id string) {
	// Desconecta todas as portas primeiro
	agent, exists := n.agents[id]
	if !exists {
		return
	}

	// Desconecta principal
	principal := agent.GetPrincipalPort()
	if conn := n.connections[principal]; conn != nil {
		n.disconnect(principal, *conn)
	}

	// Desconecta auxiliares
	for _, aux := range agent.GetAuxiliaryPorts() {
		if conn := n.connections[aux]; conn != nil {
			n.disconnect(aux, *conn)
		}
	}

	delete(n.agents, id)
	n.removeActivePairsForAgent(id)
	n.UpdatedAt = time.Now()
}

func (n *Net) Connect(from, to PortID) (ActivePair, bool, error) {
	// Valida existência
	fromAgent, ok := n.agents[from.AgentID]
	if !ok {
		return ActivePair{}, false, fmt.Errorf("agent %s not found", from.AgentID)
	}
	toAgent, ok := n.agents[to.AgentID]
	if !ok {
		return ActivePair{}, false, fmt.Errorf("agent %s not found", to.AgentID)
	}

	// Valida portas
	if _, ok := fromAgent.GetPort(from.PortName); !ok {
		return ActivePair{}, false, fmt.Errorf("port %s not found in agent %s", from.PortName, from.AgentID)
	}
	if _, ok := toAgent.GetPort(to.PortName); !ok {
		return ActivePair{}, false, fmt.Errorf("port %s not found in agent %s", to.PortName, to.AgentID)
	}

	// Verifica se já conectada
	if n.connections[from] != nil {
		return ActivePair{}, false, fmt.Errorf("port %s already connected", from)
	}
	if n.connections[to] != nil {
		return ActivePair{}, false, fmt.Errorf("port %s already connected", to)
	}

	// Conecta (bidirecional)
	n.connections[from] = &to
	n.connections[to] = &from
	n.UpdatedAt = time.Now()

	// Verifica se forma par ativo (ambas principais)
	pair, ok := n.checkActivePair(from, to)
	return pair, ok, nil
}

func (n *Net) disconnect(a, b PortID) {
	delete(n.connections, a)
	delete(n.connections, b)
}

func (n *Net) checkActivePair(a, b PortID) (ActivePair, bool) {
	if !n.isPrincipalPort(a) || !n.isPrincipalPort(b) {
		return ActivePair{}, false
	}
	pair := ActivePair{AgentAID: a.AgentID, AgentBID: b.AgentID, PortA: a, PortB: b}
	if n.hasActivePair(pair) {
		return ActivePair{}, false
	}
	n.activePairs = append(n.activePairs, pair)
	return pair, true
}

func (n *Net) GetActivePairs() []ActivePair {
	// Retorna cópia
	result := make([]ActivePair, len(n.activePairs))
	copy(result, n.activePairs)
	return result
}

func (n *Net) RemoveActivePair(idx int) {
	if idx < 0 || idx >= len(n.activePairs) {
		return
	}
	n.activePairs = append(n.activePairs[:idx], n.activePairs[idx+1:]...)
}

func (n *Net) removeActivePairsForAgent(agentID string) {
	filtered := n.activePairs[:0]
	for _, p := range n.activePairs {
		if p.AgentAID == agentID || p.AgentBID == agentID {
			continue
		}
		filtered = append(filtered, p)
	}
	n.activePairs = filtered
}

func (n *Net) hasActivePair(pair ActivePair) bool {
	for _, p := range n.activePairs {
		if (p.AgentAID == pair.AgentAID && p.AgentBID == pair.AgentBID) ||
			(p.AgentAID == pair.AgentBID && p.AgentBID == pair.AgentAID) {
			return true
		}
	}
	return false
}

func (n *Net) isPrincipalPort(p PortID) bool {
	agent, ok := n.agents[p.AgentID]
	if !ok {
		return false
	}
	return agent.GetPrincipalPort().PortName == p.PortName
}

func (n *Net) collectExternalPorts(agent Agent) map[string]*PortID {
	ext := make(map[string]*PortID)
	for _, aux := range agent.GetAuxiliaryPorts() {
		conn := n.connections[aux]
		if conn == nil {
			ext[aux.PortName] = nil
			continue
		}
		c := *conn
		ext[aux.PortName] = &c
	}
	return ext
}

func (n *Net) applyRule(
	ctx context.Context,
	pair ActivePair,
	rule Rule,
	agentA Agent,
	agentB Agent,
	extA map[string]*PortID,
	extB map[string]*PortID,
	registry AgentRegistry,
) (*InteractionResult, error) {
	// Valida interface: cada porta externa deve ser usada exatamente uma vez
	usedExternal := make(map[string]bool)
	for _, iface := range rule.Interface {
		if iface.External.Side != "A" && iface.External.Side != "B" {
			return nil, fmt.Errorf("invalid external side: %s", iface.External.Side)
		}
		key := iface.External.Side + ":" + iface.External.Port
		if usedExternal[key] {
			return nil, fmt.Errorf("external port used twice: %s", key)
		}
		usedExternal[key] = true
	}

	for port := range extA {
		key := "A:" + port
		if !usedExternal[key] {
			return nil, fmt.Errorf("missing interface for external port %s", key)
		}
	}
	for port := range extB {
		key := "B:" + port
		if !usedExternal[key] {
			return nil, fmt.Errorf("missing interface for external port %s", key)
		}
	}

	// Instancia novos agentes
	idMap := make(map[string]string)
	newAgents := make([]Agent, 0, len(rule.Agents))
	inheritedA := false
	inheritedB := false
	for _, ra := range rule.Agents {
		newID := fmt.Sprintf("%s_%s_%s", pair.AgentAID, pair.AgentBID, ra.ID)
		idMap[ra.ID] = newID
		cfg := ra.Config
		if !inheritedA && ra.Type == agentA.GetType() {
			if extCfg := agentA.GetConfig(); len(extCfg) != 0 {
				cfg = extCfg
				inheritedA = true
			}
		}
		if !inheritedB && ra.Type == agentB.GetType() {
			if extCfg := agentB.GetConfig(); len(extCfg) != 0 {
				cfg = extCfg
				inheritedB = true
			}
		}
		snap := AgentSnapshot{
			ID:        newID,
			Type:      ra.Type,
			State:     StateIdle.String(),
			Config:    cfg,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if ra.Type == agentA.GetType() && agentA.GetInput() != nil {
			snap.Input = agentA.GetInput()
		}
		if ra.Type == agentB.GetType() && agentB.GetInput() != nil {
			snap.Input = agentB.GetInput()
		}
		agent, err := registry.Create(snap)
		if err != nil {
			return nil, err
		}
		newAgents = append(newAgents, agent)
	}

	agentByID := make(map[string]Agent)
	newIDToAgent := make(map[string]Agent)
	for i, ra := range rule.Agents {
		agentByID[ra.ID] = newAgents[i]
		newIDToAgent[idMap[ra.ID]] = newAgents[i]
	}

	// Valida e cria conexões internas
	var newConnections []Connection
	usedInternal := make(map[string]bool)
	for _, rc := range rule.Connections {
		fromAgent, ok := agentByID[rc.From.AgentID]
		if !ok {
			return nil, fmt.Errorf("unknown rule agent: %s", rc.From.AgentID)
		}
		toAgent, ok := agentByID[rc.To.AgentID]
		if !ok {
			return nil, fmt.Errorf("unknown rule agent: %s", rc.To.AgentID)
		}
		if _, ok := fromAgent.GetPort(rc.From.Port); !ok {
			return nil, fmt.Errorf("port %s not found in agent %s", rc.From.Port, rc.From.AgentID)
		}
		if _, ok := toAgent.GetPort(rc.To.Port); !ok {
			return nil, fmt.Errorf("port %s not found in agent %s", rc.To.Port, rc.To.AgentID)
		}
		fromKey := rc.From.AgentID + ":" + rc.From.Port
		toKey := rc.To.AgentID + ":" + rc.To.Port
		if usedInternal[fromKey] || usedInternal[toKey] {
			return nil, fmt.Errorf("internal port used twice: %s or %s", fromKey, toKey)
		}
		usedInternal[fromKey] = true
		usedInternal[toKey] = true
		newConnections = append(newConnections, Connection{
			From: PortID{AgentID: idMap[rc.From.AgentID], PortName: rc.From.Port},
			To:   PortID{AgentID: idMap[rc.To.AgentID], PortName: rc.To.Port},
		})
	}

	// Conexões da interface (liga portas externas aos novos agentes)
	for _, iface := range rule.Interface {
		internalKey := iface.Internal.AgentID + ":" + iface.Internal.Port
		if usedInternal[internalKey] {
			return nil, fmt.Errorf("internal port used twice: %s", internalKey)
		}
		usedInternal[internalKey] = true

		var ext *PortID
		if iface.External.Side == "A" {
			ext = extA[iface.External.Port]
		} else {
			ext = extB[iface.External.Port]
		}

		internalAgent, ok := agentByID[iface.Internal.AgentID]
		if !ok {
			return nil, fmt.Errorf("unknown rule agent: %s", iface.Internal.AgentID)
		}
		if _, ok := internalAgent.GetPort(iface.Internal.Port); !ok {
			return nil, fmt.Errorf("port %s not found in agent %s", iface.Internal.Port, iface.Internal.AgentID)
		}
		if ext != nil {
			newConnections = append(newConnections, Connection{
				From: *ext,
				To:   PortID{AgentID: idMap[iface.Internal.AgentID], PortName: iface.Internal.Port},
			})
		}
	}

	// Condição 4: RHS não pode conter pares ativos (entre novos agentes)
	for _, conn := range newConnections {
		fromAgent, fromNew := newIDToAgent[conn.From.AgentID]
		toAgent, toNew := newIDToAgent[conn.To.AgentID]
		if !fromNew || !toNew {
			continue
		}
		if fromAgent.GetPrincipalPort().PortName == conn.From.PortName &&
			toAgent.GetPrincipalPort().PortName == conn.To.PortName {
			return nil, fmt.Errorf("rule produces alive pair in RHS")
		}
	}

	return &InteractionResult{
		Remove:         []string{agentA.GetID(), agentB.GetID()},
		Spawn:          newAgents,
		NewConnections: newConnections,
	}, nil
}

// ReducePair aplica regra de interação (interaction nets) para um par ativo.
func (n *Net) ReducePair(ctx context.Context, pair ActivePair, rules *RuleRegistry, registry AgentRegistry) (*InteractionResult, error) {
	agentA, ok := n.agents[pair.AgentAID]
	if !ok {
		return nil, fmt.Errorf("agent %s not found", pair.AgentAID)
	}
	agentB, ok := n.agents[pair.AgentBID]
	if !ok {
		return nil, fmt.Errorf("agent %s not found", pair.AgentBID)
	}

	// Verifica estados
	if agentA.GetState() != StateIdle && agentA.GetState() != StateWaiting {
		return nil, fmt.Errorf("agent %s not ready (state: %s)", pair.AgentAID, agentA.GetState())
	}
	if agentB.GetState() != StateIdle && agentB.GetState() != StateWaiting {
		return nil, fmt.Errorf("agent %s not ready (state: %s)", pair.AgentBID, agentB.GetState())
	}

	principalA := agentA.GetPrincipalPort()
	principalB := agentB.GetPrincipalPort()
	if conn := n.connections[principalA]; conn == nil || conn.AgentID != principalB.AgentID || conn.PortName != principalB.PortName {
		return nil, fmt.Errorf("agents %s and %s are not an active pair", pair.AgentAID, pair.AgentBID)
	}

	if rules == nil {
		return nil, fmt.Errorf("rule registry is required")
	}

	rule, swapped, ok := rules.Find(agentA.GetType(), agentB.GetType())
	if !ok {
		return nil, fmt.Errorf("no rule for %s,%s", agentA.GetType(), agentB.GetType())
	}

	actualA := agentA
	actualB := agentB
	if swapped {
		actualA, actualB = agentB, agentA
	}

	extA := n.collectExternalPorts(actualA)
	extB := n.collectExternalPorts(actualB)

	result, err := n.applyRule(ctx, pair, rule, actualA, actualB, extA, extB, registry)
	if err != nil {
		return nil, err
	}
	n.UpdatedAt = time.Now()
	return result, nil
}

// Snapshot serializa o net inteiro
func (n *Net) Snapshot() (NetSnapshot, error) {
	snapshot := NetSnapshot{
		ID:          n.ID,
		UserID:      n.UserID,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
		Connections: make([]Connection, 0),
		ActivePairs: n.activePairs,
	}

	// Serializa agentes
	for _, agent := range n.agents {
		agentSnap, err := agent.Serialize()
		if err != nil {
			return NetSnapshot{}, err
		}
		snapshot.Agents = append(snapshot.Agents, agentSnap)
	}

	// Serializa conexões (evita duplicatas)
	seen := make(map[string]bool)
	for from, to := range n.connections {
		key := from.String() + "-" + to.String()
		reverse := to.String() + "-" + from.String()
		if seen[key] || seen[reverse] {
			continue
		}
		seen[key] = true

		snapshot.Connections = append(snapshot.Connections, Connection{
			From: from,
			To:   *to,
		})
	}

	return snapshot, nil
}

// Restore reconstrói net de snapshot
func RestoreFromSnapshot(snapshot NetSnapshot, registry AgentRegistry) (*Net, error) {
	net := NewNet(snapshot.ID, snapshot.UserID)
	net.CreatedAt = snapshot.CreatedAt
	net.UpdatedAt = snapshot.UpdatedAt
	net.activePairs = snapshot.ActivePairs

	// Restaura agentes
	for _, agentSnap := range snapshot.Agents {
		agent, err := registry.Create(agentSnap)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent %s: %w", agentSnap.ID, err)
		}
		if err := net.AddAgent(agent); err != nil {
			return nil, err
		}
	}

	// Restaura conexões
	for _, conn := range snapshot.Connections {
		if _, _, err := net.Connect(conn.From, conn.To); err != nil {
			return nil, fmt.Errorf("failed to connect %s to %s: %w", conn.From, conn.To, err)
		}
	}

	return net, nil
}
