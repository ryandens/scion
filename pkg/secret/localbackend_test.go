// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !no_sqlite

package secret

import (
	"context"
	"testing"

	"github.com/ptone/scion-agent/pkg/store"
	"github.com/ptone/scion-agent/pkg/store/sqlite"
)

func createTestBackend(t *testing.T) SecretBackend {
	t.Helper()
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate test store: %v", err)
	}
	return NewLocalBackend(s)
}

func TestLocalBackend_SetAndGet(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	input := &SetSecretInput{
		Name:        "API_KEY",
		Value:       "sk-test-123",
		SecretType:  TypeEnvironment,
		Target:      "API_KEY",
		Scope:       ScopeUser,
		ScopeID:     "user-1",
		Description: "Test API key",
	}

	created, meta, err := backend.Set(ctx, input)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if !created {
		t.Error("expected created=true for new secret")
	}
	if meta.Name != "API_KEY" {
		t.Errorf("expected name %q, got %q", "API_KEY", meta.Name)
	}
	if meta.Version != 1 {
		t.Errorf("expected version 1, got %d", meta.Version)
	}

	// Get it back with value
	sv, err := backend.Get(ctx, "API_KEY", ScopeUser, "user-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if sv.Value != "sk-test-123" {
		t.Errorf("expected value %q, got %q", "sk-test-123", sv.Value)
	}
	if sv.SecretType != TypeEnvironment {
		t.Errorf("expected type %q, got %q", TypeEnvironment, sv.SecretType)
	}
}

func TestLocalBackend_SetUpdate(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	input := &SetSecretInput{
		Name:       "API_KEY",
		Value:      "old-value",
		SecretType: TypeEnvironment,
		Scope:      ScopeUser,
		ScopeID:    "user-1",
	}

	_, _, err := backend.Set(ctx, input)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Update
	input.Value = "new-value"
	created, meta, err := backend.Set(ctx, input)
	if err != nil {
		t.Fatalf("Set update failed: %v", err)
	}
	if created {
		t.Error("expected created=false for update")
	}
	if meta.Version != 2 {
		t.Errorf("expected version 2, got %d", meta.Version)
	}

	sv, err := backend.Get(ctx, "API_KEY", ScopeUser, "user-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if sv.Value != "new-value" {
		t.Errorf("expected value %q, got %q", "new-value", sv.Value)
	}
}

func TestLocalBackend_Delete(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	input := &SetSecretInput{
		Name:       "TO_DELETE",
		Value:      "value",
		SecretType: TypeEnvironment,
		Scope:      ScopeUser,
		ScopeID:    "user-1",
	}

	_, _, err := backend.Set(ctx, input)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if err := backend.Delete(ctx, "TO_DELETE", ScopeUser, "user-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = backend.Get(ctx, "TO_DELETE", ScopeUser, "user-1")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestLocalBackend_DeleteNotFound(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	err := backend.Delete(ctx, "NONEXISTENT", ScopeUser, "user-1")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalBackend_List(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	for _, name := range []string{"A_KEY", "B_KEY", "C_KEY"} {
		_, _, err := backend.Set(ctx, &SetSecretInput{
			Name:       name,
			Value:      "val-" + name,
			SecretType: TypeEnvironment,
			Scope:      ScopeUser,
			ScopeID:    "user-1",
		})
		if err != nil {
			t.Fatalf("Set %s failed: %v", name, err)
		}
	}

	metas, err := backend.List(ctx, Filter{Scope: ScopeUser, ScopeID: "user-1"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 3 {
		t.Errorf("expected 3 secrets, got %d", len(metas))
	}
}

func TestLocalBackend_ListFilterByType(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "ENV_KEY",
		Value:      "val",
		SecretType: TypeEnvironment,
		Scope:      ScopeUser,
		ScopeID:    "user-1",
	})
	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "FILE_KEY",
		Value:      "data",
		SecretType: TypeFile,
		Target:     "/tmp/file",
		Scope:      ScopeUser,
		ScopeID:    "user-1",
	})

	metas, err := backend.List(ctx, Filter{Scope: ScopeUser, ScopeID: "user-1", Type: TypeFile})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 1 {
		t.Errorf("expected 1 file secret, got %d", len(metas))
	}
	if metas[0].Name != "FILE_KEY" {
		t.Errorf("expected FILE_KEY, got %s", metas[0].Name)
	}
}

func TestLocalBackend_GetMeta(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	_, _, err := backend.Set(ctx, &SetSecretInput{
		Name:       "META_KEY",
		Value:      "secret-value",
		SecretType: TypeVariable,
		Target:     "config",
		Scope:      ScopeGrove,
		ScopeID:    "grove-1",
	})
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	meta, err := backend.GetMeta(ctx, "META_KEY", ScopeGrove, "grove-1")
	if err != nil {
		t.Fatalf("GetMeta failed: %v", err)
	}
	if meta.Name != "META_KEY" {
		t.Errorf("expected name %q, got %q", "META_KEY", meta.Name)
	}
	if meta.SecretType != TypeVariable {
		t.Errorf("expected type %q, got %q", TypeVariable, meta.SecretType)
	}
}

func TestLocalBackend_Resolve(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	// User-level secrets
	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "API_KEY",
		Value:      "user-api-key",
		SecretType: TypeEnvironment,
		Scope:      ScopeUser,
		ScopeID:    "user-1",
	})
	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "TLS_CERT",
		Value:      "cert-data",
		SecretType: TypeFile,
		Target:     "/etc/ssl/cert.pem",
		Scope:      ScopeUser,
		ScopeID:    "user-1",
	})

	// Grove-level override
	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "API_KEY",
		Value:      "grove-api-key",
		SecretType: TypeEnvironment,
		Scope:      ScopeGrove,
		ScopeID:    "grove-1",
	})
	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "DB_PASS",
		Value:      "grove-db-pass",
		SecretType: TypeEnvironment,
		Target:     "DATABASE_PASSWORD",
		Scope:      ScopeGrove,
		ScopeID:    "grove-1",
	})

	resolved, err := backend.Resolve(ctx, "user-1", "grove-1", "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	byName := make(map[string]SecretWithValue)
	for _, sv := range resolved {
		byName[sv.Name] = sv
	}

	// API_KEY overridden by grove
	apiKey, ok := byName["API_KEY"]
	if !ok {
		t.Fatal("expected API_KEY in resolved secrets")
	}
	if apiKey.Value != "grove-api-key" {
		t.Errorf("expected grove API_KEY value %q, got %q", "grove-api-key", apiKey.Value)
	}
	if apiKey.Scope != ScopeGrove {
		t.Errorf("expected API_KEY scope %q, got %q", ScopeGrove, apiKey.Scope)
	}

	// TLS_CERT from user (no override)
	cert, ok := byName["TLS_CERT"]
	if !ok {
		t.Fatal("expected TLS_CERT in resolved secrets")
	}
	if cert.SecretType != TypeFile {
		t.Errorf("expected TLS_CERT type %q, got %q", TypeFile, cert.SecretType)
	}
	if cert.Target != "/etc/ssl/cert.pem" {
		t.Errorf("expected TLS_CERT target %q, got %q", "/etc/ssl/cert.pem", cert.Target)
	}

	// DB_PASS from grove
	dbPass, ok := byName["DB_PASS"]
	if !ok {
		t.Fatal("expected DB_PASS in resolved secrets")
	}
	if dbPass.Target != "DATABASE_PASSWORD" {
		t.Errorf("expected DB_PASS target %q, got %q", "DATABASE_PASSWORD", dbPass.Target)
	}

	if len(resolved) != 3 {
		t.Errorf("expected 3 resolved secrets, got %d", len(resolved))
	}
}

func TestLocalBackend_ResolveNoScopes(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	resolved, err := backend.Resolve(ctx, "", "", "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(resolved) != 0 {
		t.Errorf("expected 0 resolved secrets, got %d", len(resolved))
	}
}

func TestLocalBackend_ResolveBrokerOverride(t *testing.T) {
	backend := createTestBackend(t)
	ctx := context.Background()

	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "API_KEY",
		Value:      "user-key",
		SecretType: TypeEnvironment,
		Scope:      ScopeUser,
		ScopeID:    "user-1",
	})
	_, _, _ = backend.Set(ctx, &SetSecretInput{
		Name:       "API_KEY",
		Value:      "broker-key",
		SecretType: TypeEnvironment,
		Scope:      ScopeRuntimeBroker,
		ScopeID:    "broker-1",
	})

	resolved, err := backend.Resolve(ctx, "user-1", "", "broker-1")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved secret, got %d", len(resolved))
	}
	if resolved[0].Value != "broker-key" {
		t.Errorf("expected broker override %q, got %q", "broker-key", resolved[0].Value)
	}
	if resolved[0].Scope != ScopeRuntimeBroker {
		t.Errorf("expected scope %q, got %q", ScopeRuntimeBroker, resolved[0].Scope)
	}
}
