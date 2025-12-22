package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ptone/gswarm/pkg/util"
)

type AppleContainerRuntime struct {
	Command string // usually "container"
}

func NewAppleContainerRuntime() *AppleContainerRuntime {
	return &AppleContainerRuntime{
		Command: "container",
	}
}

func (r *AppleContainerRuntime) Run(ctx context.Context, config RunConfig) (string, error) {
	args := []string{"run", "-d"}
	args = append(args, "-t", "--name", config.Name)

	// container CLI doesn't support --init

	if config.HomeDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/home/node", config.HomeDir))
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
		if config.HomeDir != "" {
			// Copy OAuth creds file to home dir
			dst := filepath.Join(config.HomeDir, ".gemini", "oauth_creds.json")
			if err := util.CopyFile(config.Auth.OAuthCreds, dst); err != nil {
				return "", fmt.Errorf("failed to copy OAuth creds: %w", err)
			}
		} else {
			// Fallback to mount if no home dir (might fail on some runtimes)
			containerPath := "/home/node/.gemini/oauth_creds.json"
			args = append(args, "-v", fmt.Sprintf("%s:%s:ro", config.Auth.OAuthCreds, containerPath))
		}
		args = append(args, "-e", "GEMINI_DEFAULT_AUTH_TYPE=oauth-personal")
	}
	if config.Auth.GoogleCloudProject != "" {
		args = append(args, "-e", fmt.Sprintf("GOOGLE_CLOUD_PROJECT=%s", config.Auth.GoogleCloudProject))
	}
	if config.Auth.GoogleAppCredentials != "" {
		containerPath := "/home/node/.config/gcp/application_default_credentials.json"
		if config.HomeDir != "" {
			// Copy ADC file to home dir
			dst := filepath.Join(config.HomeDir, ".config", "gcp", "application_default_credentials.json")
			if err := util.CopyFile(config.Auth.GoogleAppCredentials, dst); err != nil {
				return "", fmt.Errorf("failed to copy ADC: %w", err)
			}
		} else {
			// Fallback to mount if no home dir (might fail on some runtimes)
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
		args = append(args, "-v", fmt.Sprintf("%s:/home/node/.config/gcloud:ro", gcloudConfigDir))
	}

	for _, e := range config.Env {
		args = append(args, "-e", e)
	}

	for k, v := range config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}
	if config.UseTmux {
		args = append(args, "--label", "gswarm.tmux=true")
	}

	args = append(args, config.Image)

	if config.UseTmux {
		// When using tmux, we pass a single string as the command to new-session.
		// We must quote the task to ensure it's treated as one argument by the shell inside tmux.
		geminiCmd := fmt.Sprintf("gemini --yolo --prompt-interactive %q", config.Task)
		args = append(args, "tmux", "new-session", "-s", "gswarm", geminiCmd)
	} else {
		// When not using tmux, we pass arguments directly to container run.
		args = append(args, "gemini", "--yolo", "--prompt-interactive", config.Task)
	}

	if os.Getenv("GSWARM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "Debug: %s %s\n", r.Command, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, r.Command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("container run failed: %w (output: %s)", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *AppleContainerRuntime) Stop(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, r.Command, "stop", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("container stop failed: %w (output: %s)", err, string(out))
	}

	cmdRm := exec.CommandContext(ctx, r.Command, "rm", id)
	outRm, err := cmdRm.CombinedOutput()
	if err != nil {
		return fmt.Errorf("container rm failed: %w (output: %s)", err, string(outRm))
	}

	return nil
}

type containerListOutput struct {
	Status        string `json:"status"`
	Configuration struct {
		ID     string            `json:"id"`
		Labels map[string]string `json:"labels"`
		Image  struct {
			Reference string `json:"reference"`
		} `json:"image"`
	} `json:"configuration"`
}

func (r *AppleContainerRuntime) List(ctx context.Context, labelFilter map[string]string) ([]AgentInfo, error) {
	args := []string{"list", "-a", "--format", "json"}

	cmd := exec.CommandContext(ctx, r.Command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("container list failed: %w (output: %s)", err, string(out))
	}

	var raw []containerListOutput
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse container list output: %w", err)
	}

	// fmt.Printf("Raw containers: %d\n", len(raw))

	var agents []AgentInfo
	for _, c := range raw {
		// fmt.Printf("Checking container %s, labels: %+v\n", c.Configuration.ID, c.Configuration.Labels)
		// Filter by labels if requested
		if len(labelFilter) > 0 {
			match := true
			for k, v := range labelFilter {
				if lv, ok := c.Configuration.Labels[k]; !ok || lv != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		agents = append(agents, AgentInfo{
			ID:     c.Configuration.ID,
			Name:   c.Configuration.Labels["gswarm.name"],
			Status: c.Status,
			Image:  c.Configuration.Image.Reference,
		})
	}

	return agents, nil
}

func (r *AppleContainerRuntime) GetLogs(ctx context.Context, id string) (string, error) {
	cmd := exec.CommandContext(ctx, r.Command, "logs", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("container logs failed: %w (output: %s)", err, string(out))
	}
	return string(out), nil
}

func (r *AppleContainerRuntime) Attach(ctx context.Context, id string) error {
	// Find the container to check for tmux label
	agents, err := r.List(ctx, nil)
	useTmux := false
	if err == nil {
		for _, a := range agents {
			if a.ID == id || a.Name == id {
				// We need labels here, but AgentInfo doesn't have them.
				// Let's re-run list with format json to be sure or just try tmux.
				break
			}
		}
	}

	// For Apple Container, we highly recommend tmux.
	// We'll try to detect it by running a quick exec.
	checkTmux := exec.CommandContext(ctx, r.Command, "exec", id, "tmux", "ls")
	if err := checkTmux.Run(); err == nil {
		useTmux = true
	}

	if !useTmux {
		return fmt.Errorf("apple 'container' runtime requires tmux to attach to an interactive session. Please ensure the agent was started with tmux support")
	}

	args := []string{"exec", "-it", id, "tmux", "attach", "-t", "gswarm"}

	cmd := exec.Command(r.Command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *AppleContainerRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, r.Command, "image", "inspect", image)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}
