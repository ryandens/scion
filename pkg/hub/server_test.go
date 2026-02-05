//go:build !no_sqlite

package hub

import (
	"context"
	"testing"

	"github.com/ptone/scion-agent/pkg/store/sqlite"
)

func TestServer_PersistentSigningKeys(t *testing.T) {
	// Create an in-memory SQLite store
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate test store: %v", err)
	}

	cfg := DefaultServerConfig()

	// Create first server
	srv1 := New(cfg, s)
	if srv1.agentTokenService == nil {
		t.Fatal("agentTokenService not initialized in srv1")
	}
	if srv1.userTokenService == nil {
		t.Fatal("userTokenService not initialized in srv1")
	}

	key1 := srv1.agentTokenService.config.SigningKey
	userKey1 := srv1.userTokenService.config.SigningKey

	// Create second server with the same store
	srv2 := New(cfg, s)
	if srv2.agentTokenService == nil {
		t.Fatal("agentTokenService not initialized in srv2")
	}
	if srv2.userTokenService == nil {
		t.Fatal("userTokenService not initialized in srv2")
	}

	key2 := srv2.agentTokenService.config.SigningKey
	userKey2 := srv2.userTokenService.config.SigningKey

	// Check if keys match
	if string(key1) != string(key2) {
		t.Errorf("agent signing keys do not match: %x != %x", key1, key2)
	}
	if string(userKey1) != string(userKey2) {
		t.Errorf("user signing keys do not match: %x != %x", userKey1, userKey2)
	}
}
