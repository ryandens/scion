package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ptone/scion/pkg/config"
)

func TestBuildCommonRunArgs(t *testing.T) {
	tmpHome := t.TempDir()
	tmpWorkspace := t.TempDir()

	// Setup some dummy auth files
	tmpDir := t.TempDir()
	oauthFile := filepath.Join(tmpDir, "oauth.json")
	os.WriteFile(oauthFile, []byte("{}"), 0644)
	adcFile := filepath.Join(tmpDir, "adc.json")
	os.WriteFile(adcFile, []byte("{}"), 0644)

	tests := []struct {
		name    string
		config  RunConfig
		wantIn  []string
		wantOut []string
	}{
		{
			name: "basic config",
			config: RunConfig{
				Name:         "test-agent",
				UnixUsername: "scion",
				Image:        "scion-agent:latest",
				Task:         "hello",
			},
			wantIn: []string{"run", "-d", "-t", "--name", "test-agent", "scion-agent:latest", "gemini", "hello"},
		},
		{
			name: "workspace and home",
			config: RunConfig{
				Name:         "test-agent",
				UnixUsername: "scion",
				Image:        "scion-agent:latest",
				HomeDir:      tmpHome,
				Workspace:    tmpWorkspace,
				Task:         "hello",
			},
			wantIn: []string{
				"-v", fmt.Sprintf("%s:/home/scion", tmpHome),
				"-v", fmt.Sprintf("%s:/workspace", tmpWorkspace),
				"--workdir", "/workspace",
			},
		},
		{
			name: "gemini api key",
			config: RunConfig{
				Name: "test-agent",
				Auth: config.AuthConfig{
					GeminiAPIKey: "sk-123",
				},
				Image: "scion-agent:latest",
			},
			wantIn: []string{"-e", "GEMINI_API_KEY=sk-123", "-e", "GEMINI_DEFAULT_AUTH_TYPE=gemini-api-key"},
		},
		{
			name: "labels and tmux",
			config: RunConfig{
				Name: "test-agent",
				Labels: map[string]string{
					"foo": "bar",
				},
				UseTmux: true,
				Image:   "scion-agent:latest",
				Task:    "hello",
			},
			wantIn: []string{
				"--label", "foo=bar",
				"--label", "scion.tmux=true",
				"tmux", "new-session", "-s", "scion",
			},
		},
		{
			name: "oauth propagation with home",
			config: RunConfig{
				Name:         "test-agent",
				UnixUsername: "scion",
				HomeDir:      tmpHome,
				Auth: config.AuthConfig{
					OAuthCreds: oauthFile,
				},
				Image: "scion-agent:latest",
			},
			wantIn: []string{"-e", "GEMINI_DEFAULT_AUTH_TYPE=oauth-personal"},
		},
		{
			name: "adc propagation without home",
			config: RunConfig{
				Name:         "test-agent",
				UnixUsername: "scion",
				Auth: config.AuthConfig{
					GoogleAppCredentials: adcFile,
				},
				Image: "scion-agent:latest",
			},
			wantIn: []string{
				"-v", fmt.Sprintf("%s:/home/scion/.config/gcp/application_default_credentials.json:ro", adcFile),
				"-e", "GOOGLE_APPLICATION_CREDENTIALS=/home/scion/.config/gcp/application_default_credentials.json",
				"-e", "GEMINI_DEFAULT_AUTH_TYPE=compute-default-credentials",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := buildCommonRunArgs(tt.config)
			if err != nil {
				t.Fatalf("buildCommonRunArgs failed: %v", err)
			}
			argStr := strings.Join(args, " ")
			for _, want := range tt.wantIn {
				if !strings.Contains(argStr, want) {
					t.Errorf("expected arg %q not found in %v", want, args)
				}
			}
		})
	}
}