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
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ptone/scion-agent/pkg/api"
	codexEmbeds "github.com/ptone/scion-agent/pkg/harness/codex"
)

type Codex struct{}

func (c *Codex) Name() string {
	return "codex"
}

func (c *Codex) DiscoverAuth(agentHome string) api.AuthConfig {
	auth := api.AuthConfig{}
	// Check for Codex auth file in standard location
	home, _ := os.UserHomeDir()
	authPath := filepath.Join(home, ".codex", "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		auth.CodexAuthFile = authPath
	}
	return auth
}

func (c *Codex) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	return map[string]string{}
}

func (c *Codex) GetCommand(task string, resume bool, baseArgs []string) []string {
	args := []string{"codex", "--yolo"}
	if resume {
		args = append(args, "resume", "--last")
	} else {
		if task != "" {
			args = append(args, task)
		}
	}

	args = append(args, baseArgs...)
	return args
}

func (c *Codex) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	return nil
}

func (c *Codex) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	return nil
}

func (c *Codex) DefaultConfigDir() string {
	return ""
}

func (c *Codex) HasSystemPrompt(agentHome string) bool {
	return false
}

func (c *Codex) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	auth := c.DiscoverAuth(agentHome)
	return c.PropagateFiles(agentHome, "", auth)
}

func (c *Codex) GetEmbedDir() string {
	return "codex"
}

func (c *Codex) GetInterruptKey() string {
	return "C-c"
}

func (c *Codex) GetHarnessEmbedsFS() (embed.FS, string) {
	return codexEmbeds.EmbedsFS, "embeds"
}

func (c *Codex) GetTelemetryEnv() map[string]string {
	// Codex uses a TOML config file for telemetry, not env vars.
	// File-based injection is handled via PropagateFiles.
	return nil
}

func (c *Codex) InjectAgentInstructions(agentHome string, content []byte) error {
	dir := filepath.Join(agentHome, ".codex")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create .codex directory: %w", err)
	}
	target := filepath.Join(dir, "AGENTS.md")
	return os.WriteFile(target, content, 0644)
}

func (c *Codex) RequiredEnvKeys(authSelectedType string) []string {
	return nil
}

func (c *Codex) ResolveAuth(auth api.AuthConfig) (*api.ResolvedAuth, error) {
	// Preference order: CodexAPIKey → OpenAIAPIKey → CodexAuthFile → error

	if auth.CodexAPIKey != "" {
		return &api.ResolvedAuth{
			Method: "codex-api-key",
			EnvVars: map[string]string{
				"CODEX_API_KEY": auth.CodexAPIKey,
			},
		}, nil
	}

	if auth.OpenAIAPIKey != "" {
		return &api.ResolvedAuth{
			Method: "openai-api-key",
			EnvVars: map[string]string{
				"OPENAI_API_KEY": auth.OpenAIAPIKey,
			},
		}, nil
	}

	if auth.CodexAuthFile != "" {
		return &api.ResolvedAuth{
			Method: "codex-auth-file",
			Files: []api.FileMapping{
				{
					SourcePath:    auth.CodexAuthFile,
					ContainerPath: "~/.codex/auth.json",
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("codex: no valid auth method found; set CODEX_API_KEY or OPENAI_API_KEY, or provide auth credentials at ~/.codex/auth.json")
}

func (c *Codex) InjectSystemPrompt(agentHome string, content []byte) error {
	// TODO: Codex has no native system prompt support. System prompt injection is
	// not yet implemented for this harness.
	return nil
}
