package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ptone/scion/pkg/util"
)

// buildCommonRunArgs constructs the common arguments for 'run' command across different runtimes.
func buildCommonRunArgs(config RunConfig) ([]string, error) {
	args := []string{"run", "-d", "-t"}
	args = append(args, "--name", config.Name)

	if config.HomeDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/home/%s", config.HomeDir, config.UnixUsername))
	}
	if config.Workspace != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/workspace", config.Workspace))
		args = append(args, "--workdir", "/workspace")
	}

	// Propagate Auth
	if config.Auth.GeminiAPIKey != "" {
		args = append(args, "-e", fmt.Sprintf("GEMINI_API_KEY=%s", config.Auth.GeminiAPIKey))
		args = append(args, "-e", "GEMINI_DEFAULT_AUTH_TYPE=gemini-api-key")
	}
	if config.Auth.GoogleAPIKey != "" {
		args = append(args, "-e", fmt.Sprintf("GOOGLE_API_KEY=%s", config.Auth.GoogleAPIKey))
		args = append(args, "-e", "GEMINI_DEFAULT_AUTH_TYPE=gemini-api-key")
	}
	if config.Auth.VertexAPIKey != "" {
		args = append(args, "-e", fmt.Sprintf("VERTEX_API_KEY=%s", config.Auth.VertexAPIKey))
		args = append(args, "-e", "GEMINI_DEFAULT_AUTH_TYPE=vertex-ai")
	}
	if config.Auth.OAuthCreds != "" {
		containerPath := fmt.Sprintf("/home/%s/.gemini/oauth_creds.json", config.UnixUsername)
		if config.HomeDir != "" {
			dst := filepath.Join(config.HomeDir, ".gemini", "oauth_creds.json")
			if err := util.CopyFile(config.Auth.OAuthCreds, dst); err != nil {
				return nil, fmt.Errorf("failed to copy OAuth creds: %w", err)
			}
		} else {
			args = append(args, "-v", fmt.Sprintf("%s:%s:ro", config.Auth.OAuthCreds, containerPath))
		}
		args = append(args, "-e", "GEMINI_DEFAULT_AUTH_TYPE=oauth-personal")
	}
	if config.Auth.GoogleCloudProject != "" {
		args = append(args, "-e", fmt.Sprintf("GOOGLE_CLOUD_PROJECT=%s", config.Auth.GoogleCloudProject))
	}
	if config.Auth.GoogleAppCredentials != "" {
		containerPath := fmt.Sprintf("/home/%s/.config/gcp/application_default_credentials.json", config.UnixUsername)
		if config.HomeDir != "" {
			dst := filepath.Join(config.HomeDir, ".config", "gcp", "application_default_credentials.json")
			if err := util.CopyFile(config.Auth.GoogleAppCredentials, dst); err != nil {
				return nil, fmt.Errorf("failed to copy ADC: %w", err)
			}
		} else {
			args = append(args, "-v", fmt.Sprintf("%s:%s:ro", config.Auth.GoogleAppCredentials, containerPath))
		}
		args = append(args, "-e", fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS=%s", containerPath))
		args = append(args, "-e", "GEMINI_DEFAULT_AUTH_TYPE=compute-default-credentials")
	}

	if config.Model != "" {
		args = append(args, "-e", fmt.Sprintf("GEMINI_MODEL=%s", config.Model))
	}

	// Mount gcloud config if it exists
	home, _ := os.UserHomeDir()
	gcloudConfigDir := filepath.Join(home, ".config", "gcloud")
	if _, err := os.Stat(gcloudConfigDir); err == nil {
		args = append(args, "-v", fmt.Sprintf("%s:/home/%s/.config/gcloud:ro", gcloudConfigDir, config.UnixUsername))
	}

	for _, e := range config.Env {
		args = append(args, "-e", e)
	}

	for k, v := range config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}
	if config.UseTmux {
		args = append(args, "--label", "scion.tmux=true")
	}

	args = append(args, config.Image)

	if config.UseTmux {
		geminiCmd := fmt.Sprintf("gemini --yolo --prompt-interactive %q", config.Task)
		args = append(args, "tmux", "new-session", "-s", "scion", geminiCmd)
	} else {
		args = append(args, "gemini", "--yolo", "--prompt-interactive", config.Task)
	}

	return args, nil
}

func runSimpleCommand(ctx context.Context, command string, args ...string) (string, error) {
	if os.Getenv("SCION_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "Debug: %s %s\n", command, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s failed: %w", command, args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func runInteractiveCommand(command string, args ...string) error {
	if os.Getenv("SCION_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "Debug: %s %s\n", command, strings.Join(args, " "))
	}
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
