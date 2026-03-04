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
	opencodeEmbeds "github.com/ptone/scion-agent/pkg/harness/opencode"
)

type OpenCode struct{}

func (o *OpenCode) Name() string {
	return "opencode"
}

func (o *OpenCode) DiscoverAuth(agentHome string) api.AuthConfig {
	auth := api.AuthConfig{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
	}
	// Check for OpenCode auth file in standard location
	home, _ := os.UserHomeDir()
	authPath := filepath.Join(home, ".local", "share", "opencode", "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		auth.OpenCodeAuthFile = authPath
	}
	return auth
}

func (o *OpenCode) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	return map[string]string{}
}

func (o *OpenCode) GetCommand(task string, resume bool, baseArgs []string) []string {
	args := []string{"opencode"}
	if resume {
		args = append(args, "--continue")
	} else {
		args = append(args, "--prompt")
		if task != "" {
			args = append(args, task)
		}
	}

	args = append(args, baseArgs...)
	return args
}
func (o *OpenCode) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	return nil
}

func (o *OpenCode) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	return nil
}

func (o *OpenCode) DefaultConfigDir() string {
	return ".config/opencode"
}

func (o *OpenCode) HasSystemPrompt(agentHome string) bool {
	return false
}

func (o *OpenCode) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	auth := o.DiscoverAuth(agentHome)
	return o.PropagateFiles(agentHome, "", auth)
}

func (o *OpenCode) GetEmbedDir() string {
	return "opencode"
}

func (o *OpenCode) GetInterruptKey() string {
	return "C-c"
}

func (o *OpenCode) GetHarnessEmbedsFS() (embed.FS, string) {
	return opencodeEmbeds.EmbedsFS, "embeds"
}

func (o *OpenCode) GetTelemetryEnv() map[string]string {
	// OpenCode telemetry env var injection is deferred.
	return nil
}

func (o *OpenCode) InjectAgentInstructions(agentHome string, content []byte) error {
	target := filepath.Join(agentHome, "AGENTS.md")
	return os.WriteFile(target, content, 0644)
}

func (o *OpenCode) RequiredEnvKeys(authSelectedType string) []string {
	return []string{"ANTHROPIC_API_KEY"}
}

func (o *OpenCode) ResolveAuth(auth api.AuthConfig) (*api.ResolvedAuth, error) {
	// Preference order: AnthropicAPIKey → OpenAIAPIKey → OpenCodeAuthFile → error

	if auth.AnthropicAPIKey != "" {
		return &api.ResolvedAuth{
			Method: "anthropic-api-key",
			EnvVars: map[string]string{
				"ANTHROPIC_API_KEY": auth.AnthropicAPIKey,
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

	if auth.OpenCodeAuthFile != "" {
		return &api.ResolvedAuth{
			Method: "opencode-auth-file",
			Files: []api.FileMapping{
				{
					SourcePath:    auth.OpenCodeAuthFile,
					ContainerPath: "~/.local/share/opencode/auth.json",
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("opencode: no valid auth method found; set ANTHROPIC_API_KEY or OPENAI_API_KEY, or provide auth credentials at ~/.local/share/opencode/auth.json")
}

func (o *OpenCode) InjectSystemPrompt(agentHome string, content []byte) error {
	// OpenCode has no native system prompt support — downgrade by prepending to AGENTS.md
	agentsPath := filepath.Join(agentHome, "AGENTS.md")
	header := fmt.Sprintf("# System Prompt\n\n%s\n\n---\n\n", string(content))

	existing, err := os.ReadFile(agentsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing agent instructions: %w", err)
	}

	merged := []byte(header)
	if len(existing) > 0 {
		merged = append(merged, existing...)
	}
	return os.WriteFile(agentsPath, merged, 0644)
}
