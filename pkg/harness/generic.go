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
	"github.com/ptone/scion-agent/pkg/config"
)

type Generic struct{}

func (g *Generic) Name() string {
	return "generic"
}

func (g *Generic) DiscoverAuth(agentHome string) api.AuthConfig {
	auth := api.AuthConfig{
		GoogleAppCredentials: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		GoogleCloudProject:   os.Getenv("GOOGLE_CLOUD_PROJECT"),
	}

	if auth.GoogleCloudProject == "" {
		auth.GoogleCloudProject = os.Getenv("GCP_PROJECT")
	}

	// Check agent settings (from template)
	agentSettingsPath := filepath.Join(agentHome, g.DefaultConfigDir(), "settings.json")
	if agentSettings, err := config.LoadAgentSettings(agentSettingsPath); err == nil {
		if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" && agentSettings.ApiKey != "" {
			// Determine where to put the API key.
			// Since we don't know the harness, we might not be able to assign it correctly
			// if it's not one of the known ones.
			// However, AgentSettings struct is somewhat tailored to Gemini currently.
			// We'll leave it as is for now or maybe try to guess?
			// For generic, if ApiKey is there, maybe we put it in GeminiAPIKey as a fallback,
			// or maybe we need a GenericAPIKey field in AuthConfig?
			// Given AuthConfig limitations, we'll skip assigning it to a specific field
			// if we are not sure, or default to one if it seems appropriate.
			// But for "generic", maybe we just ignore settings.json specific keys unless we know what they are.
		}
	}

	// Check for OAuth creds in default location
	home, _ := os.UserHomeDir()
	oauthPath := filepath.Join(home, g.DefaultConfigDir(), "oauth_creds.json")
	if _, err := os.Stat(oauthPath); err == nil {
		auth.OAuthCreds = oauthPath
	}

	return auth
}

func (g *Generic) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	env := make(map[string]string)
	env["SCION_AGENT_NAME"] = agentName
	return env
}

func (g *Generic) GetCommand(task string, resume bool, baseArgs []string) []string {
	args := append([]string{}, baseArgs...)
	if task != "" {
		args = append(args, task)
	}
	return args
}

func (g *Generic) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	return nil
}

func (g *Generic) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	return nil
}

func (g *Generic) DefaultConfigDir() string {
	return ".scion"
}

func (g *Generic) HasSystemPrompt(agentHome string) bool {
	return false
}

func (g *Generic) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	return nil
}

func (g *Generic) GetEmbedDir() string {
	return ""
}

func (g *Generic) GetInterruptKey() string {
	return "C-c"
}

func (g *Generic) GetHarnessEmbedsFS() (embed.FS, string) {
	return embed.FS{}, ""
}

func (g *Generic) GetTelemetryEnv() map[string]string {
	return nil
}

func (g *Generic) InjectAgentInstructions(agentHome string, content []byte) error {
	target := filepath.Join(agentHome, "agents.md")
	return os.WriteFile(target, content, 0644)
}

func (g *Generic) RequiredEnvKeys(authSelectedType string) []string {
	return nil
}

func (g *Generic) ResolveAuth(auth api.AuthConfig) (*api.ResolvedAuth, error) {
	// Generic harness is a passthrough — map all available creds, never error.
	result := &api.ResolvedAuth{
		Method:  "passthrough",
		EnvVars: make(map[string]string),
	}

	if auth.AnthropicAPIKey != "" {
		result.EnvVars["ANTHROPIC_API_KEY"] = auth.AnthropicAPIKey
	}
	if auth.GeminiAPIKey != "" {
		result.EnvVars["GEMINI_API_KEY"] = auth.GeminiAPIKey
	}
	if auth.GoogleAPIKey != "" {
		result.EnvVars["GOOGLE_API_KEY"] = auth.GoogleAPIKey
	}
	if auth.OpenAIAPIKey != "" {
		result.EnvVars["OPENAI_API_KEY"] = auth.OpenAIAPIKey
	}
	if auth.CodexAPIKey != "" {
		result.EnvVars["CODEX_API_KEY"] = auth.CodexAPIKey
	}
	if auth.GoogleCloudProject != "" {
		result.EnvVars["GOOGLE_CLOUD_PROJECT"] = auth.GoogleCloudProject
	}
	if auth.GoogleCloudRegion != "" {
		result.EnvVars["GOOGLE_CLOUD_REGION"] = auth.GoogleCloudRegion
	}

	if auth.GoogleAppCredentials != "" {
		adcContainerPath := "~/.config/gcp/application_default_credentials.json"
		result.EnvVars["GOOGLE_APPLICATION_CREDENTIALS"] = adcContainerPath
		result.Files = append(result.Files, api.FileMapping{
			SourcePath:    auth.GoogleAppCredentials,
			ContainerPath: adcContainerPath,
		})
	}
	if auth.OAuthCreds != "" {
		result.Files = append(result.Files, api.FileMapping{
			SourcePath:    auth.OAuthCreds,
			ContainerPath: "~/.scion/oauth_creds.json",
		})
	}
	if auth.CodexAuthFile != "" {
		result.Files = append(result.Files, api.FileMapping{
			SourcePath:    auth.CodexAuthFile,
			ContainerPath: "~/.codex/auth.json",
		})
	}
	if auth.OpenCodeAuthFile != "" {
		result.Files = append(result.Files, api.FileMapping{
			SourcePath:    auth.OpenCodeAuthFile,
			ContainerPath: "~/.local/share/opencode/auth.json",
		})
	}

	return result, nil
}

func (g *Generic) InjectSystemPrompt(agentHome string, content []byte) error {
	target := filepath.Join(agentHome, ".scion", "system_prompt.md")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("failed to create directory for system prompt: %w", err)
	}
	return os.WriteFile(target, content, 0644)
}
