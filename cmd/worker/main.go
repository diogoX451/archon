package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	eventadapter "github.com/diogoX451/archon/internal/adapters/events"
	"github.com/diogoX451/archon/internal/adapters/store"
	"github.com/diogoX451/archon/internal/agents"
	"github.com/diogoX451/archon/internal/config"
	"github.com/diogoX451/archon/internal/core/domain"
	"github.com/diogoX451/archon/internal/core/ports"
	"github.com/diogoX451/archon/internal/core/service"
	"github.com/diogoX451/archon/internal/events"
	natsevents "github.com/diogoX451/archon/internal/events/nats"
	redisstore "github.com/diogoX451/archon/internal/store/redis"
	"github.com/diogoX451/archon/pkg/types"
)

func main() {
	// Config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Infra
	redisClient, err := redisstore.New(redisstore.Config{
		Addr:       cfg.Redis.Addr,
		DefaultTTL: 24 * time.Hour,
	})
	if err != nil {
		log.Fatal("redis:", err)
	}

	natsBus, err := natsevents.New(natsevents.Config{
		URL: cfg.NATS.URL,
	})
	if err != nil {
		log.Fatal("nats:", err)
	}

	if err := natsBus.SetupArchonStreams(); err != nil {
		log.Fatal("streams:", err)
	}

	registry := agents.NewRegistry()
	agents.RegisterBuiltins(registry)
	rules := domain.NewRuleRegistry()

	// Adapters (conectam infra com core)
	netRepo := store.NewNetRepository(redisClient, registry)
	eventBus := eventadapter.NewEventBus(natsBus)

	// Core service
	executor := service.NewExecutor(netRepo, eventBus, registry, rules)

	// Worker loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logEvents := strings.TrimSpace(types.Getenv("ARCHON_LOG_EVENTS", "")) != ""

	if err := loadRules(ctx, redisClient, rules, registry); err != nil {
		log.Fatal("load rules:", err)
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Inicia consumo
	log.Println("Worker started:", cfg.App.WorkerID)

	if err := runWorker(ctx, executor, eventBus, natsBus, redisClient, registry, rules, logEvents); err != nil {
		log.Fatal(err)
	}
}

func runWorker(
	ctx context.Context,
	executor *service.Executor,
	bus ports.EventBus,
	natsBus *natsevents.NATSBus,
	store *redisstore.RedisStore,
	registry domain.AgentRegistry,
	rules *domain.RuleRegistry,
	logEvents bool,
) error {
	locker := newNetLocker()
	maxParallel := readMaxParallelPairs()

	// Handler para interações
	interactionHandler := func(ctx context.Context, netID string, pair domain.ActivePair) error {
		if logEvents {
			log.Printf("interaction pending net=%s pair=%s:%s", netID, pair.AgentAID, pair.AgentBID)
		}
		unlock := locker.Lock(netID)
		defer unlock()

		lockToken, ok, err := store.AcquireNetLock(ctx, netID, readNetLockTTL())
		if err != nil {
			log.Printf("net lock error net=%s err=%v", netID, err)
			return err
		}
		if !ok {
			if logEvents {
				log.Printf("net locked net=%s", netID)
			}
			return fmt.Errorf("net locked: %s", netID)
		}
		defer func() {
			if err := store.ReleaseNetLock(ctx, netID, lockToken); err != nil && logEvents {
				log.Printf("net unlock error net=%s err=%v", netID, err)
			}
		}()

		if err := executor.ExecutePendingPairs(ctx, netID, maxParallel); err != nil {
			log.Printf("interaction error net=%s err=%v", netID, err)
			return err
		}
		return nil
	}

	// Handler para respostas
	responseHandler := func(ctx context.Context, correlationID string, data domain.Data) error {
		if logEvents {
			log.Printf("response received correlation_id=%s bytes=%d", correlationID, len(data))
		}
		// Encontra qual agente estava esperando
		netID, agentID, err := executor.FindWaitingAgent(ctx, correlationID)
		if err != nil {
			return err
		}
		return executor.ResumeAgent(ctx, netID, agentID, data)
	}

	commandHandler := func(ctx context.Context, msg events.Message) error {
		var event types.SpawnEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			return err
		}
		if logEvents {
			log.Printf("command spawn workflow=%s agents=%d conns=%d", event.WorkflowID, len(event.Blueprint.Agents), len(event.Blueprint.Connections))
		}
		if err := handleSpawn(ctx, store, bus, registry, rules, event); err != nil {
			return err
		}
		return msg.Ack()
	}

	addAgentHandler := func(ctx context.Context, msg events.Message) error {
		var event types.AddAgentEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			return err
		}
		if logEvents {
			log.Printf("command add_agent workflow=%s agent=%s", event.WorkflowID, event.Agent.ID)
		}
		if err := handleAddAgent(ctx, store, event); err != nil {
			return err
		}
		return msg.Ack()
	}

	connectHandler := func(ctx context.Context, msg events.Message) error {
		var event types.ConnectEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			return err
		}
		if logEvents {
			log.Printf("command connect workflow=%s from=%s:%s to=%s:%s", event.WorkflowID, event.From.AgentID, event.From.Port, event.To.AgentID, event.To.Port)
		}
		if err := handleConnect(ctx, store, bus, registry, rules, event); err != nil {
			return err
		}
		return msg.Ack()
	}

	defineRuleHandler := func(ctx context.Context, msg events.Message) error {
		var event types.DefineRuleEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			return err
		}
		if logEvents {
			log.Printf("command define_rule %s|%s", event.Rule.AgentAType, event.Rule.AgentBType)
		}
		if err := handleDefineRule(ctx, rules, store, registry, event); err != nil {
			return err
		}
		return msg.Ack()
	}

	// Subscreve em ambos
	if err := bus.SubscribeInteractions(ctx, interactionHandler); err != nil {
		return err
	}
	if err := bus.SubscribeResponses(ctx, responseHandler); err != nil {
		return err
	}
	if _, err := natsBus.Subscribe("archon.command.spawn", commandHandler); err != nil {
		return err
	}
	if _, err := natsBus.Subscribe("archon.command.add_agent", addAgentHandler); err != nil {
		return err
	}
	if _, err := natsBus.Subscribe("archon.command.connect", connectHandler); err != nil {
		return err
	}
	if _, err := natsBus.Subscribe("archon.command.define_rule", defineRuleHandler); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

type netLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newNetLocker() *netLocker {
	return &netLocker{
		locks: make(map[string]*sync.Mutex),
	}
}

func (l *netLocker) Lock(netID string) func() {
	l.mu.Lock()
	lock, ok := l.locks[netID]
	if !ok {
		lock = &sync.Mutex{}
		l.locks[netID] = lock
	}
	l.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}

func readMaxParallelPairs() int {
	raw := strings.TrimSpace(types.Getenv("ARCHON_MAX_PARALLEL_PAIRS", "4"))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 1
	}
	return value
}

func readNetLockTTL() time.Duration {
	raw := strings.TrimSpace(types.Getenv("ARCHON_NET_LOCK_TTL_SECONDS", "30"))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 30 * time.Second
	}
	return time.Duration(value) * time.Second
}

func handleSpawn(
	ctx context.Context,
	store *redisstore.RedisStore,
	bus ports.EventBus,
	registry domain.AgentRegistry,
	rules *domain.RuleRegistry,
	event types.SpawnEvent,
) error {
	now := time.Now()

	incoming := make(map[string]bool, len(event.Blueprint.Agents))
	for _, c := range event.Blueprint.Connections {
		to := parsePortRef(c.To)
		if to.AgentID != "" {
			incoming[to.AgentID] = true
		}
	}

	agents := make(map[types.AgentID]*types.AgentSnapshot, len(event.Blueprint.Agents))
	for _, a := range event.Blueprint.Agents {
		input := types.Data(nil)
		if !incoming[a.ID] && event.Input != nil {
			input = event.Input
		}
		agents[types.AgentID(a.ID)] = &types.AgentSnapshot{
			ID:        types.AgentID(a.ID),
			Type:      a.Type,
			State:     types.StateIdle,
			Config:    a.Config,
			Input:     input,
			UpdatedAt: now,
		}
	}

	state := &types.WorkflowState{
		ID:          types.WorkflowID(event.WorkflowID),
		UserID:      "",
		Status:      types.WorkflowRunning,
		CreatedAt:   event.CreatedAt,
		UpdatedAt:   now,
		Agents:      agents,
		Connections: []types.ConnectionSnapshot{},
	}

	for _, c := range event.Blueprint.Connections {
		from := parsePortRef(c.From)
		to := parsePortRef(c.To)
		if from.Port == "" {
			from.Port = principalPortName(registry, agents, from.AgentID)
		}
		if to.Port == "" {
			to.Port = principalPortName(registry, agents, to.AgentID)
		}

		state.Connections = append(state.Connections, types.ConnectionSnapshot{
			From:      types.AgentID(from.AgentID),
			FromPort:  from.Port,
			To:        types.AgentID(to.AgentID),
			ToPort:    to.Port,
			CreatedAt: now,
		})
	}

	if err := store.CreateWorkflow(ctx, state); err != nil {
		return err
	}

	for _, conn := range state.Connections {
		agentA := state.Agents[conn.From]
		agentB := state.Agents[conn.To]
		if agentA == nil || agentB == nil {
			continue
		}
		if isPrincipal(registry, *agentA, conn.FromPort) && isPrincipal(registry, *agentB, conn.ToPort) {
			if err := ensureRuleForPair(rules, agentA.Type, agentB.Type); err != nil {
				return err
			}
			if err := bus.PublishInteraction(ctx, event.WorkflowID, domain.ActivePair{
				AgentAID: string(conn.From),
				AgentBID: string(conn.To),
				PortA:    domain.PortID{AgentID: string(conn.From), PortName: conn.FromPort},
				PortB:    domain.PortID{AgentID: string(conn.To), PortName: conn.ToPort},
			}); err != nil {
				return err
			}
		}
	}

	// Execução direta para fluxo trivial (1 agente, sem conexões)
	if len(event.Blueprint.Agents) == 1 && len(event.Blueprint.Connections) == 0 {
		agentID := event.Blueprint.Agents[0].ID
		snap := domain.AgentSnapshot{
			ID:        agentID,
			Type:      event.Blueprint.Agents[0].Type,
			State:     types.StateIdle.String(),
			Config:    event.Blueprint.Agents[0].Config,
			Input:     event.Input,
			CreatedAt: now,
			UpdatedAt: now,
		}
		agent, err := registry.Create(snap)
		if err != nil {
			return err
		}

		result := agent.Execute(ctx)
		updated := &types.AgentSnapshot{
			ID:        types.AgentID(agentID),
			Type:      snap.Type,
			State:     mapState(result.State),
			Config:    snap.Config,
			Input:     event.Input,
			Output:    result.Output,
			Need:      mapNeed(result.Need),
			UpdatedAt: time.Now(),
		}
		if err := store.UpdateAgent(ctx, types.WorkflowID(event.WorkflowID), updated); err != nil {
			return err
		}

		if result.State == domain.StateWaiting && result.Need != nil {
			need := mapNeed(result.Need)
			if need != nil {
				if err := store.RegisterWaiting(ctx, types.WorkflowID(event.WorkflowID), types.AgentID(agentID), need); err != nil {
					return err
				}
				return bus.PublishNeed(ctx, event.WorkflowID, *result.Need)
			}
		}

		if result.State == domain.StateCompleted {
			return bus.PublishResult(ctx, event.WorkflowID, result.Output)
		}
	}

	return nil
}

func parsePortRef(value string) types.PortRef {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) == 1 {
		return types.PortRef{AgentID: parts[0]}
	}
	return types.PortRef{AgentID: parts[0], Port: parts[1]}
}

func principalPortName(registry domain.AgentRegistry, agents map[types.AgentID]*types.AgentSnapshot, agentID string) string {
	agentSnap := agents[types.AgentID(agentID)]
	if agentSnap == nil {
		return ""
	}
	agent := toDomainAgent(registry, *agentSnap)
	if agent == nil {
		return ""
	}
	return agent.GetPrincipalPort().PortName
}

func toDomainAgent(registry domain.AgentRegistry, snap types.AgentSnapshot) domain.Agent {
	agentSnap := domain.AgentSnapshot{
		ID:        string(snap.ID),
		Type:      snap.Type,
		State:     snap.State.String(),
		Config:    snap.Config,
		Input:     snap.Input,
		Output:    snap.Output,
		CreatedAt: snap.UpdatedAt,
		UpdatedAt: snap.UpdatedAt,
	}
	agent, err := registry.Create(agentSnap)
	if err != nil {
		return nil
	}
	return agent
}

func loadRules(ctx context.Context, store *redisstore.RedisStore, rules *domain.RuleRegistry, registry domain.AgentRegistry) error {
	list, err := store.ListRules(ctx)
	if err != nil {
		return err
	}
	for _, rule := range list {
		r, err := registerRule(registry, rule)
		if err != nil {
			return err
		}
		if err := rules.Register(r); err != nil {
			return err
		}
	}
	return nil
}

func handleAddAgent(ctx context.Context, store *redisstore.RedisStore, event types.AddAgentEvent) error {
	state, err := store.GetWorkflow(ctx, types.WorkflowID(event.WorkflowID))
	if err != nil {
		return err
	}

	if state.Agents == nil {
		state.Agents = make(map[types.AgentID]*types.AgentSnapshot)
	}

	agentID := types.AgentID(event.Agent.ID)
	if _, exists := state.Agents[agentID]; exists {
		return fmt.Errorf("agent %s already exists", event.Agent.ID)
	}

	state.Agents[agentID] = &types.AgentSnapshot{
		ID:        agentID,
		Type:      event.Agent.Type,
		State:     types.StateIdle,
		Config:    event.Agent.Config,
		UpdatedAt: time.Now(),
	}

	return store.UpdateWorkflow(ctx, state)
}

func handleConnect(
	ctx context.Context,
	store *redisstore.RedisStore,
	bus ports.EventBus,
	registry domain.AgentRegistry,
	rules *domain.RuleRegistry,
	event types.ConnectEvent,
) error {
	state, err := store.GetWorkflow(ctx, types.WorkflowID(event.WorkflowID))
	if err != nil {
		return err
	}

	state.Connections = append(state.Connections, types.ConnectionSnapshot{
		From:      types.AgentID(event.From.AgentID),
		FromPort:  event.From.Port,
		To:        types.AgentID(event.To.AgentID),
		ToPort:    event.To.Port,
		CreatedAt: time.Now(),
	})

	if err := store.UpdateWorkflow(ctx, state); err != nil {
		return err
	}

	// Se conexão for entre portas principais, publica interação
	agentA, ok := state.Agents[types.AgentID(event.From.AgentID)]
	if !ok {
		return fmt.Errorf("agent %s not found", event.From.AgentID)
	}
	agentB, ok := state.Agents[types.AgentID(event.To.AgentID)]
	if !ok {
		return fmt.Errorf("agent %s not found", event.To.AgentID)
	}

	if isPrincipal(registry, *agentA, event.From.Port) && isPrincipal(registry, *agentB, event.To.Port) {
		if err := ensureRuleForPair(rules, agentA.Type, agentB.Type); err != nil {
			return err
		}
		return bus.PublishInteraction(ctx, event.WorkflowID, domain.ActivePair{
			AgentAID: event.From.AgentID,
			AgentBID: event.To.AgentID,
			PortA:    domain.PortID{AgentID: event.From.AgentID, PortName: event.From.Port},
			PortB:    domain.PortID{AgentID: event.To.AgentID, PortName: event.To.Port},
		})
	}

	return nil
}

func handleDefineRule(
	ctx context.Context,
	rules *domain.RuleRegistry,
	store *redisstore.RedisStore,
	registry domain.AgentRegistry,
	event types.DefineRuleEvent,
) error {
	rule, err := registerRule(registry, event.Rule)
	if err != nil {
		return err
	}
	if err := rules.Register(rule); err != nil {
		return err
	}

	return store.SaveRule(ctx, event.Rule)
}

func registerRule(registry domain.AgentRegistry, ruleDef types.RuleDef) (domain.Rule, error) {
	if err := validateRule(registry, ruleDef); err != nil {
		return domain.Rule{}, err
	}

	rule := domain.Rule{
		AgentAType: ruleDef.AgentAType,
		AgentBType: ruleDef.AgentBType,
	}

	for _, a := range ruleDef.Agents {
		rule.Agents = append(rule.Agents, domain.RuleAgent{
			ID:     a.ID,
			Type:   a.Type,
			Config: a.Config,
		})
	}

	for _, c := range ruleDef.Connections {
		rule.Connections = append(rule.Connections, domain.RuleConnection{
			From: domain.RulePortRef{AgentID: c.From.AgentID, Port: c.From.Port},
			To:   domain.RulePortRef{AgentID: c.To.AgentID, Port: c.To.Port},
		})
	}

	for _, iface := range ruleDef.Interface {
		rule.Interface = append(rule.Interface, domain.RuleInterface{
			External: domain.RuleExternalPort{Side: iface.External.Side, Port: iface.External.Port},
			Internal: domain.RulePortRef{AgentID: iface.Internal.AgentID, Port: iface.Internal.Port},
		})
	}

	return rule, nil
}

func isPrincipal(registry domain.AgentRegistry, snap types.AgentSnapshot, port string) bool {
	agentSnap := domain.AgentSnapshot{
		ID:        string(snap.ID),
		Type:      snap.Type,
		State:     snap.State.String(),
		Config:    snap.Config,
		Input:     snap.Input,
		Output:    snap.Output,
		CreatedAt: snap.UpdatedAt,
		UpdatedAt: snap.UpdatedAt,
	}
	agent, err := registry.Create(agentSnap)
	if err != nil {
		return false
	}
	return agent.GetPrincipalPort().PortName == port
}

func validateRule(registry domain.AgentRegistry, rule types.RuleDef) error {
	if rule.AgentAType == "" || rule.AgentBType == "" {
		return fmt.Errorf("agent types are required")
	}
	if rule.AgentAType == rule.AgentBType {
		return fmt.Errorf("no rule for identical symbols: %s", rule.AgentAType)
	}

	if err := validateExternalPorts(registry, rule.AgentAType, rule.Interface, "A"); err != nil {
		return err
	}
	if err := validateExternalPorts(registry, rule.AgentBType, rule.Interface, "B"); err != nil {
		return err
	}

	internalUsed := make(map[string]bool)
	for _, conn := range rule.Connections {
		if err := validateRulePort(registry, conn.From.AgentID, conn.From.Port, rule.Agents); err != nil {
			return err
		}
		if err := validateRulePort(registry, conn.To.AgentID, conn.To.Port, rule.Agents); err != nil {
			return err
		}
		fromKey := conn.From.AgentID + ":" + conn.From.Port
		toKey := conn.To.AgentID + ":" + conn.To.Port
		if internalUsed[fromKey] || internalUsed[toKey] {
			return fmt.Errorf("internal port used twice: %s or %s", fromKey, toKey)
		}
		internalUsed[fromKey] = true
		internalUsed[toKey] = true
	}

	for _, iface := range rule.Interface {
		key := iface.Internal.AgentID + ":" + iface.Internal.Port
		if internalUsed[key] {
			return fmt.Errorf("internal port used twice: %s", key)
		}
		internalUsed[key] = true
		if err := validateRulePort(registry, iface.Internal.AgentID, iface.Internal.Port, rule.Agents); err != nil {
			return err
		}
	}

	return nil
}

func validateExternalPorts(registry domain.AgentRegistry, agentType string, iface []types.RuleInterfaceDef, side string) error {
	agent := toDomainAgent(registry, types.AgentSnapshot{
		ID:        types.AgentID("tmp"),
		Type:      agentType,
		State:     types.StateIdle,
		UpdatedAt: time.Now(),
	})
	if agent == nil {
		return fmt.Errorf("unknown agent type: %s", agentType)
	}

	principal := agent.GetPrincipalPort().PortName
	aux := make(map[string]bool)
	for _, p := range agent.GetAuxiliaryPorts() {
		aux[p.PortName] = true
	}

	used := make(map[string]bool)
	for _, entry := range iface {
		if entry.External.Side != side {
			continue
		}
		if entry.External.Port == principal {
			return fmt.Errorf("interface cannot use principal port %s:%s", side, principal)
		}
		if !aux[entry.External.Port] {
			return fmt.Errorf("unknown external port %s:%s", side, entry.External.Port)
		}
		if used[entry.External.Port] {
			return fmt.Errorf("external port used twice: %s:%s", side, entry.External.Port)
		}
		used[entry.External.Port] = true
	}

	for port := range aux {
		if !used[port] {
			return fmt.Errorf("missing interface for external port %s:%s", side, port)
		}
	}

	return nil
}

func validateRulePort(registry domain.AgentRegistry, agentID, port string, agents []types.AgentDef) error {
	var agentType string
	var agentConfig types.Data
	for _, a := range agents {
		if a.ID == agentID {
			agentType = a.Type
			agentConfig = a.Config
			break
		}
	}
	if agentType == "" {
		return fmt.Errorf("unknown rule agent: %s", agentID)
	}

	agent := toDomainAgent(registry, types.AgentSnapshot{
		ID:        types.AgentID(agentID),
		Type:      agentType,
		State:     types.StateIdle,
		Config:    agentConfig,
		UpdatedAt: time.Now(),
	})
	if agent == nil {
		return fmt.Errorf("unknown agent type: %s", agentType)
	}

	if _, ok := agent.GetPort(port); !ok {
		return fmt.Errorf("port %s not found in agent %s", port, agentID)
	}

	return nil
}

func ensureRuleForPair(rules *domain.RuleRegistry, aType, bType string) error {
	if rules == nil {
		return fmt.Errorf("rule registry not available")
	}
	if _, _, ok := rules.Find(aType, bType); !ok {
		return fmt.Errorf("no rule for %s,%s", aType, bType)
	}
	return nil
}

func mapState(state domain.AgentState) types.AgentState {
	switch state {
	case domain.StateIdle:
		return types.StateIdle
	case domain.StateRunning:
		return types.StateRunning
	case domain.StateWaiting:
		return types.StateWaiting
	case domain.StateCompleted:
		return types.StateCompleted
	case domain.StateFailed:
		return types.StateFailed
	default:
		return types.StateIdle
	}
}

func mapNeed(need *domain.ExternalNeed) *types.ExternalNeed {
	if need == nil {
		return nil
	}
	return &types.ExternalNeed{
		Type:          need.Type,
		CorrelationID: need.CorrelationID,
		Payload:       need.Payload,
		CreatedAt:     need.CreatedAt,
	}
}
