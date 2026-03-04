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

package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ptone/scion-agent/pkg/api"
)

func TestGeneric_Name(t *testing.T) {
	g := &Generic{}
	if g.Name() != "generic" {
		t.Errorf("Expected name 'generic', got '%s'", g.Name())
	}
}

func TestGeneric_DiscoverAuth(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "test-gemini-key")
	os.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	defer os.Unsetenv("GEMINI_API_KEY")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	g := &Generic{}
	auth := g.DiscoverAuth("/tmp")

	// Implicit env discovery is removed, so these should be empty
	if auth.GeminiAPIKey != "" {
		t.Errorf("Expected GeminiAPIKey '', got '%s'", auth.GeminiAPIKey)
	}
	if auth.AnthropicAPIKey != "" {
		t.Errorf("Expected AnthropicAPIKey '', got '%s'", auth.AnthropicAPIKey)
	}
}

func TestGeneric_GetEnv(t *testing.T) {
	g := &Generic{}
	auth := api.AuthConfig{
		GeminiAPIKey:         "test-gemini-key",
		AnthropicAPIKey:      "test-anthropic-key",
		GoogleAppCredentials: "/path/to/creds.json",
	}

	env := g.GetEnv("test-agent", "", "test-user", auth)

	// GetEnv should only return non-auth env vars
	if env["SCION_AGENT_NAME"] != "test-agent" {
		t.Errorf("Expected SCION_AGENT_NAME = 'test-agent', got '%s'", env["SCION_AGENT_NAME"])
	}
	// Auth env vars should NOT be present (handled by ResolvedAuth)
	for _, key := range []string{"GEMINI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT"} {
		if _, ok := env[key]; ok {
			t.Errorf("Expected env[%s] to be absent (auth handled by ResolvedAuth), but it was present", key)
		}
	}
}

func TestGeneric_GetCommand(t *testing.T) {
	g := &Generic{}

	cmd := g.GetCommand("test task", false, nil)
	if !reflect.DeepEqual(cmd, []string{"test task"}) {
		t.Errorf("Expected command ['test task'], got %v", cmd)
	}

	cmdWithArgs := g.GetCommand("test task", false, []string{"--arg1"})
	if !reflect.DeepEqual(cmdWithArgs, []string{"--arg1", "test task"}) {
		t.Errorf("Expected command ['--arg1', 'test task'], got %v", cmdWithArgs)
	}

	cmdEmpty := g.GetCommand("", false, nil)
	if len(cmdEmpty) != 0 {
		t.Errorf("Expected empty command, got %v", cmdEmpty)
	}
}

func TestGeneric_DefaultConfigDir(t *testing.T) {
	g := &Generic{}
	if g.DefaultConfigDir() != ".scion" {
		t.Errorf("Expected DefaultConfigDir '.scion', got '%s'", g.DefaultConfigDir())
	}
}

func TestGenericInjectAgentInstructions(t *testing.T) {
	agentHome := t.TempDir()
	g := &Generic{}
	content := []byte("# Agent Instructions\nDo good work.")

	if err := g.InjectAgentInstructions(agentHome, content); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	target := filepath.Join(agentHome, "agents.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}

func TestGenericRequiredEnvKeys(t *testing.T) {
	g := &Generic{}

	got := g.RequiredEnvKeys("")
	if got != nil {
		t.Errorf("RequiredEnvKeys() = %v, want nil", got)
	}
}

func TestGenericResolveAuth_EmptyConfig(t *testing.T) {
	g := &Generic{}
	result, err := g.ResolveAuth(api.AuthConfig{})
	if err != nil {
		t.Fatalf("generic should never error: %v", err)
	}
	if result.Method != "passthrough" {
		t.Errorf("Method = %q, want %q", result.Method, "passthrough")
	}
	if len(result.EnvVars) != 0 {
		t.Errorf("expected empty EnvVars for empty config, got %v", result.EnvVars)
	}
	if len(result.Files) != 0 {
		t.Errorf("expected empty Files for empty config, got %d", len(result.Files))
	}
}

func TestGenericResolveAuth_AllCreds(t *testing.T) {
	g := &Generic{}
	auth := api.AuthConfig{
		AnthropicAPIKey:      "anthropic",
		GeminiAPIKey:         "gemini",
		GoogleAPIKey:         "google",
		OpenAIAPIKey:         "openai",
		CodexAPIKey:          "codex",
		GoogleCloudProject:   "proj",
		GoogleCloudRegion:    "region",
		GoogleAppCredentials: "/adc.json",
		OAuthCreds:           "/oauth.json",
		CodexAuthFile:        "/codex-auth.json",
		OpenCodeAuthFile:     "/opencode-auth.json",
	}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("generic should never error: %v", err)
	}

	expectedEnvVars := map[string]string{
		"ANTHROPIC_API_KEY":              "anthropic",
		"GEMINI_API_KEY":                 "gemini",
		"GOOGLE_API_KEY":                 "google",
		"OPENAI_API_KEY":                 "openai",
		"CODEX_API_KEY":                  "codex",
		"GOOGLE_CLOUD_PROJECT":           "proj",
		"GOOGLE_CLOUD_REGION":            "region",
		"GOOGLE_APPLICATION_CREDENTIALS": "~/.config/gcp/application_default_credentials.json",
	}
	for k, want := range expectedEnvVars {
		if got := result.EnvVars[k]; got != want {
			t.Errorf("EnvVars[%q] = %q, want %q", k, got, want)
		}
	}

	// Should have 4 file mappings: ADC, OAuth, Codex, OpenCode
	if len(result.Files) != 4 {
		t.Fatalf("expected 4 file mappings, got %d", len(result.Files))
	}
}

func TestGenericResolveAuth_PartialCreds(t *testing.T) {
	g := &Generic{}
	auth := api.AuthConfig{AnthropicAPIKey: "key-only"}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("generic should never error: %v", err)
	}
	if result.EnvVars["ANTHROPIC_API_KEY"] != "key-only" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", result.EnvVars["ANTHROPIC_API_KEY"], "key-only")
	}
	if len(result.EnvVars) != 1 {
		t.Errorf("expected 1 env var, got %d: %v", len(result.EnvVars), result.EnvVars)
	}
	if len(result.Files) != 0 {
		t.Errorf("expected no files, got %d", len(result.Files))
	}
}

func TestGenericInjectSystemPrompt(t *testing.T) {
	agentHome := t.TempDir()
	g := &Generic{}
	content := []byte("You are a helpful coding assistant.")

	if err := g.InjectSystemPrompt(agentHome, content); err != nil {
		t.Fatalf("InjectSystemPrompt failed: %v", err)
	}

	target := filepath.Join(agentHome, ".scion", "system_prompt.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}
