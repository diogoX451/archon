package redis

import (
	"context"
	"testing"
	"time"

	"github.com/diogoX451/archon/pkg/types"
)

func TestRedisStore(t *testing.T) {
	store, err := New(Config{
		Addr:       "localhost:6379",
		DefaultTTL: time.Hour,
	})
	if err != nil {
		t.Skip("Redis não disponível:", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("Create and Get Workflow", func(t *testing.T) {
		state := &types.WorkflowState{
			ID:        "wf_test_1",
			UserID:    "user_123",
			Status:    types.WorkflowRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Agents: map[types.AgentID]*types.AgentSnapshot{
				"agent_1": {
					ID:     "agent_1",
					Type:   "http",
					State:  types.StateIdle,
					Config: []byte(`{"url":"http://example.com"}`),
					Input:  []byte(`{"user_id":123}`),
				},
			},
			Connections: []types.ConnectionSnapshot{},
		}

		// Create
		if err := store.CreateWorkflow(ctx, state); err != nil {
			t.Fatal(err)
		}

		// Get
		retrieved, err := store.GetWorkflow(ctx, "wf_test_1")
		if err != nil {
			t.Fatal(err)
		}

		if retrieved.ID != "wf_test_1" {
			t.Errorf("expected wf_test_1, got %s", retrieved.ID)
		}

		if len(retrieved.Agents) != 1 {
			t.Errorf("expected 1 agent, got %d", len(retrieved.Agents))
		}

		// Cleanup
		store.DeleteWorkflow(ctx, "wf_test_1")
	})

	t.Run("Find Agents by State", func(t *testing.T) {
		state := &types.WorkflowState{
			ID:     "wf_test_2",
			UserID: "user_456",
			Status: types.WorkflowRunning,
			Agents: map[types.AgentID]*types.AgentSnapshot{
				"idle_agent": {
					ID:    "idle_agent",
					Type:  "calculator",
					State: types.StateIdle,
					Input: []byte(`[1,2,3]`),
				},
			},
		}

		store.CreateWorkflow(ctx, state)

		// Find idle agents
		units, err := store.FindAgentsByState(ctx, types.StateIdle, 10)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, u := range units {
			if u.WorkflowID == "wf_test_2" && u.AgentID == "idle_agent" {
				found = true
				break
			}
		}

		if !found {
			t.Error("did not find idle_agent")
		}

		store.DeleteWorkflow(ctx, "wf_test_2")
	})

	t.Run("Waiting and Resume", func(t *testing.T) {
		state := &types.WorkflowState{
			ID:     "wf_test_3",
			UserID: "user_789",
			Agents: map[types.AgentID]*types.AgentSnapshot{
				"waiting_agent": {
					ID:    "waiting_agent",
					Type:  "http",
					State: types.StateIdle,
					Input: []byte(`{}`),
				},
			},
		}

		store.CreateWorkflow(ctx, state)

		// Register as waiting
		need := &types.ExternalNeed{
			Type:          "http",
			CorrelationID: "corr_abc_123",
			Payload:       []byte(`{"method":"GET"}`),
			CreatedAt:     time.Now(),
		}

		err := store.RegisterWaiting(ctx, "wf_test_3", "waiting_agent", need)
		if err != nil {
			t.Fatal(err)
		}

		// Resolve waiting
		response := []byte(`{"status":200,"body":"ok"}`)
		unit, err := store.ResolveWaiting(ctx, "corr_abc_123", response)
		if err != nil {
			t.Fatal(err)
		}

		if unit.Operation != types.OpResume {
			t.Errorf("expected OpResume, got %s", unit.Operation)
		}

		store.DeleteWorkflow(ctx, "wf_test_3")
	})
}
