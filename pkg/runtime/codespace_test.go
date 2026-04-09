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

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCodespaceRuntime_Name(t *testing.T) {
	rt := NewCodespaceRuntime()
	if got := rt.Name(); got != "codespace" {
		t.Errorf("expected 'codespace', got %q", got)
	}
}

func TestCodespaceRuntime_ExecUser(t *testing.T) {
	rt := NewCodespaceRuntime()
	if got := rt.ExecUser(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestCodespaceRuntime_ImageExists(t *testing.T) {
	rt := NewCodespaceRuntime()
	exists, err := rt.ImageExists(context.Background(), "any-image")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !exists {
		t.Error("expected ImageExists to return true for codespace runtime")
	}
}

func TestCodespaceRuntime_PullImage(t *testing.T) {
	rt := NewCodespaceRuntime()
	if err := rt.PullImage(context.Background(), "any-image"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestExtractOwnerRepoFromRemote(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		expected string
	}{
		{
			name:     "https with .git suffix",
			remote:   "https://github.com/owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "https without .git suffix",
			remote:   "https://github.com/owner/repo",
			expected: "owner/repo",
		},
		{
			name:     "ssh format",
			remote:   "git@github.com:owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "ssh format without .git",
			remote:   "git@github.com:owner/repo",
			expected: "owner/repo",
		},
		{
			name:     "https with token",
			remote:   "https://x-access-token:ghp_TOKEN@github.com/owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "ssh:// protocol",
			remote:   "ssh://git@github.com/owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "empty",
			remote:   "",
			expected: "",
		},
		{
			name:     "invalid",
			remote:   "not-a-url",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOwnerRepoFromRemote(tt.remote)
			if got != tt.expected {
				t.Errorf("extractOwnerRepoFromRemote(%q) = %q, want %q", tt.remote, got, tt.expected)
			}
		})
	}
}

func TestCodespaceMetadata_RoundTrip(t *testing.T) {
	// Use a temp dir as the metadata directory by overriding HOME
	tmpDir := t.TempDir()
	testMeta := codespaceMetadata{
		CodespaceName: "test-codespace-abc123",
		Labels: map[string]string{
			"scion.agent":    "true",
			"scion.name":     "my-agent",
			"scion.grove":    "my-grove",
			"scion.template": "default",
		},
		Annotations: map[string]string{
			"scion.grove_path": "/tmp/test-project",
		},
		Image: "scion-agent:latest",
		Repo:  "owner/repo",
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	if err := saveCodespaceMetadata(testMeta); err != nil {
		t.Fatalf("saveCodespaceMetadata failed: %v", err)
	}

	loaded, err := loadCodespaceMetadata("test-codespace-abc123")
	if err != nil {
		t.Fatalf("loadCodespaceMetadata failed: %v", err)
	}

	if loaded.CodespaceName != testMeta.CodespaceName {
		t.Errorf("CodespaceName = %q, want %q", loaded.CodespaceName, testMeta.CodespaceName)
	}
	if loaded.Repo != testMeta.Repo {
		t.Errorf("Repo = %q, want %q", loaded.Repo, testMeta.Repo)
	}
	if loaded.Labels["scion.name"] != "my-agent" {
		t.Errorf("Labels[scion.name] = %q, want 'my-agent'", loaded.Labels["scion.name"])
	}
	if loaded.Annotations["scion.grove_path"] != "/tmp/test-project" {
		t.Errorf("Annotations[scion.grove_path] = %q, want '/tmp/test-project'", loaded.Annotations["scion.grove_path"])
	}

	// Test loadAll
	all := loadAllCodespaceMetadata()
	if _, ok := all["test-codespace-abc123"]; !ok {
		t.Error("loadAllCodespaceMetadata should include test-codespace-abc123")
	}

	// Test delete
	deleteCodespaceMetadata("test-codespace-abc123")
	if _, err := loadCodespaceMetadata("test-codespace-abc123"); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestLastNonEmptyLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single line", "codespace-name", "codespace-name"},
		{"with progress line", "✓ Codespaces usage for this repository is paid for by pixee\ncodespace-name", "codespace-name"},
		{"trailing newline", "codespace-name\n", "codespace-name"},
		{"multiple progress lines", "line1\nline2\ncodespace-name\n", "codespace-name"},
		{"empty", "", ""},
		{"only whitespace", "  \n  \n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastNonEmptyLine(tt.input)
			if got != tt.expected {
				t.Errorf("lastNonEmptyLine(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"it's quoted", "'it'\\''s quoted'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.expected {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCodespaceRuntime_ResolveRepo(t *testing.T) {
	t.Run("explicit repo config", func(t *testing.T) {
		rt := &CodespaceRuntime{Repo: "myorg/myrepo"}
		got := rt.resolveRepo(RunConfig{})
		if got != "myorg/myrepo" {
			t.Errorf("expected 'myorg/myrepo', got %q", got)
		}
	})

	t.Run("no repo, no workspace", func(t *testing.T) {
		rt := &CodespaceRuntime{}
		got := rt.resolveRepo(RunConfig{})
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

func TestCodespaceRuntime_List_MockGH(t *testing.T) {
	tmpDir := t.TempDir()
	mockGH := filepath.Join(tmpDir, "mock-gh")

	// Mock gh that returns a JSON list with one scion codespace and one non-scion
	script := `#!/bin/sh
if [ "$1" = "cs" ] && [ "$2" = "list" ]; then
  echo '[{"name":"user-repo-abc123","displayName":"scion-my-agent","state":"Available","repository":"owner/repo","machineName":"basicLinux32gb","owner":"user"},{"name":"other-cs","displayName":"my dev env","state":"Available","repository":"owner/repo","machineName":"basicLinux32gb","owner":"user"}]'
fi
`
	if err := os.WriteFile(mockGH, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}

	rt := &CodespaceRuntime{Command: mockGH}

	agents, err := rt.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "my-agent" {
		t.Errorf("expected agent name 'my-agent', got %q", agents[0].Name)
	}
	if agents[0].ContainerID != "user-repo-abc123" {
		t.Errorf("expected container ID 'user-repo-abc123', got %q", agents[0].ContainerID)
	}
	if agents[0].Runtime != "codespace" {
		t.Errorf("expected runtime 'codespace', got %q", agents[0].Runtime)
	}
}

func TestCodespaceRuntime_List_LabelFilter(t *testing.T) {
	tmpDir := t.TempDir()
	mockGH := filepath.Join(tmpDir, "mock-gh")

	script := `#!/bin/sh
if [ "$1" = "cs" ] && [ "$2" = "list" ]; then
  echo '[{"name":"cs-1","displayName":"scion-agent-a","state":"Available","repository":"owner/repo","machineName":"basicLinux32gb","owner":"user"},{"name":"cs-2","displayName":"scion-agent-b","state":"Available","repository":"owner/repo","machineName":"basicLinux32gb","owner":"user"}]'
fi
`
	if err := os.WriteFile(mockGH, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}

	// Set HOME to tmpDir so metadata is stored there
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Save metadata for cs-1 with a specific grove
	_ = saveCodespaceMetadata(codespaceMetadata{
		CodespaceName: "cs-1",
		Labels: map[string]string{
			"scion.agent": "true",
			"scion.name":  "agent-a",
			"scion.grove": "my-grove",
		},
	})
	// Save metadata for cs-2 with a different grove
	_ = saveCodespaceMetadata(codespaceMetadata{
		CodespaceName: "cs-2",
		Labels: map[string]string{
			"scion.agent": "true",
			"scion.name":  "agent-b",
			"scion.grove": "other-grove",
		},
	})

	rt := &CodespaceRuntime{Command: mockGH}

	// Filter by grove
	agents, err := rt.List(context.Background(), map[string]string{"scion.grove": "my-grove"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 filtered agent, got %d", len(agents))
	}
	if agents[0].Name != "agent-a" {
		t.Errorf("expected 'agent-a', got %q", agents[0].Name)
	}
}

func TestCodespaceRuntime_Exec_MockGH(t *testing.T) {
	tmpDir := t.TempDir()
	mockGH := filepath.Join(tmpDir, "mock-gh")

	script := `#!/bin/sh
echo "$@"
`
	if err := os.WriteFile(mockGH, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}

	rt := &CodespaceRuntime{Command: mockGH}

	out, err := rt.Exec(context.Background(), "my-codespace", []string{"tmux", "send-keys", "-t", "scion:0", "Enter"})
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	if !contains(out, "cs ssh -c my-codespace --") {
		t.Errorf("expected 'cs ssh -c my-codespace --' in output, got %q", out)
	}
	if !contains(out, "tmux send-keys -t scion:0 Enter") {
		t.Errorf("expected tmux command in output, got %q", out)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCodespaceRuntime_Stop_MockGH(t *testing.T) {
	tmpDir := t.TempDir()
	mockGH := filepath.Join(tmpDir, "mock-gh")

	script := `#!/bin/sh
echo "stopped"
`
	if err := os.WriteFile(mockGH, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}

	rt := &CodespaceRuntime{Command: mockGH}
	if err := rt.Stop(context.Background(), "my-codespace"); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestCodespaceRuntime_Delete_MockGH(t *testing.T) {
	tmpDir := t.TempDir()
	mockGH := filepath.Join(tmpDir, "mock-gh")

	script := `#!/bin/sh
echo "deleted"
`
	if err := os.WriteFile(mockGH, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}

	// Set HOME so metadata is in tmpDir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Save metadata first
	_ = saveCodespaceMetadata(codespaceMetadata{CodespaceName: "my-codespace"})

	rt := &CodespaceRuntime{Command: mockGH}
	if err := rt.Delete(context.Background(), "my-codespace"); err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	// Verify metadata was cleaned up
	if _, err := loadCodespaceMetadata("my-codespace"); err == nil {
		t.Error("expected metadata to be deleted")
	}
}
