package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DockerRuntime struct {
	Command string
}

func NewDockerRuntime() *DockerRuntime {
	return &DockerRuntime{Command: "docker"}
}

func (r *DockerRuntime) Run(ctx context.Context, config RunConfig) (string, error) {
	args := []string{"run", "-d"}
	args = append(args, "-t", "--init", "--name", config.Name)

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
		// Mount OAuth creds file
		containerPath := "/home/node/.gemini/oauth_creds.json"
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", config.Auth.OAuthCreds, containerPath))
		args = append(args, "-e", "GEMINI_DEFAULT_AUTH_TYPE=oauth-personal")
	}
	if config.Auth.GoogleCloudProject != "" {
		args = append(args, "-e", fmt.Sprintf("GOOGLE_CLOUD_PROJECT=%s", config.Auth.GoogleCloudProject))
	}
	if config.Auth.GoogleAppCredentials != "" {
		// Mount ADC file
		containerPath := "/home/node/.config/gcp/application_default_credentials.json"
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", config.Auth.GoogleAppCredentials, containerPath))
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
		// When not using tmux, we pass arguments directly to docker run.
		args = append(args, "gemini", "--yolo", "--prompt-interactive", config.Task)
	}

	cmd := exec.CommandContext(ctx, r.Command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run failed: %w (output: %s)", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *DockerRuntime) Stop(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, r.Command, "stop", id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop failed: %w (output: %s)", err, string(out))
	}
	cmdRm := exec.CommandContext(ctx, r.Command, "rm", id)
	if out, err := cmdRm.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm failed: %w (output: %s)", err, string(out))
	}
	return nil
}

type dockerListOutput struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Status string `json:"Status"`
	Image  string `json:"Image"`
	Labels string `json:"Labels"`
}

func (r *DockerRuntime) List(ctx context.Context, labelFilter map[string]string) ([]AgentInfo, error) {
	args := []string{"ps", "-a", "--format", "{{json .}}"}
	cmd := exec.CommandContext(ctx, r.Command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %w (output: %s)", err, string(out))
	}

	var agents []AgentInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var data dockerListOutput
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}

		// Parse Labels string "key1=val1,key2=val2"
		labels := make(map[string]string)
		if data.Labels != "" {
			pairs := strings.Split(data.Labels, ",")
			for _, pair := range pairs {
				kv := strings.SplitN(pair, "=", 2)
				if len(kv) == 2 {
					labels[kv[0]] = kv[1]
				}
			}
		}

		// Filter by labels if requested
		if len(labelFilter) > 0 {
			match := true
			for k, v := range labelFilter {
				if lv, ok := labels[k]; !ok || lv != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		agents = append(agents, AgentInfo{
			ID:     data.ID,
			Name:   data.Names,
			Status: data.Status,
			Image:  data.Image,
		})
	}
	return agents, nil
}

func (r *DockerRuntime) GetLogs(ctx context.Context, id string) (string, error) {
	cmd := exec.CommandContext(ctx, r.Command, "logs", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker logs failed: %w (output: %s)", err, string(out))
	}
	return string(out), nil
}

func (r *DockerRuntime) Attach(ctx context.Context, id string) error {
	// Check if the container is using tmux
	inspectCmd := exec.CommandContext(ctx, r.Command, "inspect", "--format", "{{index .Config.Labels \"gswarm.tmux\"}}", id)
	out, _ := inspectCmd.Output()
	useTmux := strings.TrimSpace(string(out)) == "true"

	var cmd *exec.Cmd
	if useTmux {
		cmd = exec.Command(r.Command, "exec", "-it", id, "tmux", "attach", "-t", "gswarm")
	} else {
		// Using exec.Command instead of exec.CommandContext for attach to allow interactive TTY
		// though CommandContext should also work if we don't cancel it.
		cmd = exec.Command(r.Command, "attach", id)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *DockerRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, r.Command, "image", "inspect", image)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}
