package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	args, err := buildCommonRunArgs(config)
	if err != nil {
		return "", err
	}

	out, err := runSimpleCommand(ctx, r.Command, args...)
	if err != nil {
		return "", fmt.Errorf("container run failed: %w (output: %s)", err, out)
	}
	return out, nil
}

func (r *AppleContainerRuntime) Stop(ctx context.Context, id string) error {
	_, err := runSimpleCommand(ctx, r.Command, "stop", id)
	return err
}

func (r *AppleContainerRuntime) Delete(ctx context.Context, id string) error {
	_, err := runSimpleCommand(ctx, r.Command, "rm", id)
	return err
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
		return nil, fmt.Errorf("failed to parse container list output: %w (output: %s)", err, string(out))
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
			ID:        c.Configuration.ID,
			Name:      c.Configuration.Labels["scion.name"],
			Grove:     c.Configuration.Labels["scion.grove"],
			GrovePath: c.Configuration.Labels["scion.grove_path"],
			Labels:    c.Configuration.Labels,
			Status:    c.Status,
			Image:     c.Configuration.Image.Reference,
		})
	}

	return agents, nil
}

func (r *AppleContainerRuntime) GetLogs(ctx context.Context, id string) (string, error) {
	return runSimpleCommand(ctx, r.Command, "logs", id)
}

func (r *AppleContainerRuntime) Attach(ctx context.Context, id string) error {
	useTmux := false

	// Try to find labels from list first
	agents, err := r.List(ctx, nil)
	if err == nil {
		for _, a := range agents {
			if a.ID == id || a.Name == id {
				if a.Labels["scion.tmux"] == "true" {
					useTmux = true
				}
				break
			}
		}
	}

	if !useTmux {
		// For Apple Container, we'll try to detect it by running a quick exec.
		checkTmux := exec.CommandContext(ctx, r.Command, "exec", id, "tmux", "ls")
		if err := checkTmux.Run(); err == nil {
			useTmux = true
		}
	}

	if !useTmux {
		return fmt.Errorf("apple 'container' runtime requires tmux to attach to an interactive session. Please ensure the agent was started with tmux support")
	}

	args := []string{"exec", "-it", id, "tmux", "attach", "-t", "scion"}

	return runInteractiveCommand(r.Command, args...)
}

func (r *AppleContainerRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	_, err := runSimpleCommand(ctx, r.Command, "image", "inspect", image)
	return err == nil, nil
}
