//go:build !no_sqlite

package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ptone/scion-agent/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentStatusUpdate_Authorization(t *testing.T) {
	srv, s := testServer(t)
	ctx := context.Background()

	// Create a grove
	grove := &store.Grove{
		ID:   "grove-1",
		Name: "Test Grove",
		Slug: "test-grove",
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create two agents
	agent1 := &store.Agent{
		ID:      "agent-1",
		AgentID: "agent-1-slug",
		Name:    "Agent 1",
		GroveID: grove.ID,
		Status:  store.AgentStatusRunning,
	}
	require.NoError(t, s.CreateAgent(ctx, agent1))

	agent2 := &store.Agent{
		ID:      "agent-2",
		AgentID: "agent-2-slug",
		Name:    "Agent 2",
		GroveID: grove.ID,
		Status:  store.AgentStatusRunning,
	}
	require.NoError(t, s.CreateAgent(ctx, agent2))

	// Get agent token service
	tokenSvc := srv.GetAgentTokenService()
	require.NotNil(t, tokenSvc)

	// Generate token for agent 1
	token1, err := tokenSvc.GenerateAgentToken(agent1.ID, grove.ID, []AgentTokenScope{ScopeAgentStatusUpdate})
	require.NoError(t, err)

	t.Run("Agent 1 can update its own status", func(t *testing.T) {
		status := store.AgentStatusUpdate{
			Status:  "idle",
			Message: "Waiting for user input",
		}
		body, _ := json.Marshal(status)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/status", bytes.NewReader(body))
		req.Header.Set("X-Scion-Agent-Token", token1)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify update in store
		updated, err := s.GetAgent(ctx, agent1.ID)
		require.NoError(t, err)
		assert.Equal(t, "idle", updated.Status)
		assert.Equal(t, "Waiting for user input", updated.Message)
	})

	t.Run("Agent 1 cannot update Agent 2's status", func(t *testing.T) {
		status := store.AgentStatusUpdate{
			Status: "error",
		}
		body, _ := json.Marshal(status)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-2/status", bytes.NewReader(body))
		req.Header.Set("X-Scion-Agent-Token", token1)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("Agent 1 cannot perform lifecycle actions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/stop", nil)
		req.Header.Set("X-Scion-Agent-Token", token1)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("User can update agent status", func(t *testing.T) {
		status := store.AgentStatusUpdate{
			Status: "running",
		}
		body, _ := json.Marshal(status)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/status", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testDevToken)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		updated, err := s.GetAgent(ctx, agent1.ID)
		require.NoError(t, err)
		assert.Equal(t, "running", updated.Status)
	})
}

func TestAgentStatusUpdate_Heartbeat(t *testing.T) {
	srv, s := testServer(t)
	ctx := context.Background()

	// Create a grove
	grove := &store.Grove{
		ID:   "grove-h",
		Name: "Heartbeat Grove",
		Slug: "heartbeat-grove",
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create an agent
	agent := &store.Agent{
		ID:      "agent-h",
		AgentID: "agent-h-slug",
		Name:    "Agent Heartbeat",
		GroveID: grove.ID,
		Status:  store.AgentStatusRunning,
	}
	require.NoError(t, s.CreateAgent(ctx, agent))

	// Record initial update time
	initial, err := s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	initialTime := initial.LastSeen

	// Small delay to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Send heartbeat
	status := store.AgentStatusUpdate{
		Status:    store.AgentStatusRunning,
		Heartbeat: true,
	}
	body, _ := json.Marshal(status)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-h/status", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testDevToken)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify update in store
	updated, err := s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.True(t, updated.LastSeen.After(initialTime), "LastSeen should be updated")
}
