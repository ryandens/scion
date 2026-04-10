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
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/scion/pkg/api"
	"github.com/GoogleCloudPlatform/scion/pkg/util"
)

// CodespaceRuntime implements the Runtime interface using GitHub Codespaces
// via the gh CLI. Each agent runs as a tmux session inside a codespace.
type CodespaceRuntime struct {
	Command          string // CLI binary, default "gh"
	Repo             string // Explicit owner/repo override
	Machine          string // Machine type (e.g. "basicLinux32gb")
	IdleTimeout      string // Idle timeout (e.g. "30m")
	RetentionPeriod  string // Retention period (e.g. "720h")
	DevcontainerPath string // Path to devcontainer.json inside the repo
}

func NewCodespaceRuntime() *CodespaceRuntime {
	return &CodespaceRuntime{
		Command: "gh",
	}
}

func (r *CodespaceRuntime) Name() string {
	return "codespace"
}

// ExecUser returns an empty string because gh cs ssh connects as the
// default codespace user; there is no --user flag to pass.
func (r *CodespaceRuntime) ExecUser() string {
	return ""
}

// codespaceMetadata is persisted locally so that List() can reconstruct
// AgentInfo labels without SSH-ing into every codespace.
type codespaceMetadata struct {
	CodespaceName string            `json:"codespace_name"`
	Labels        map[string]string `json:"labels"`
	Annotations   map[string]string `json:"annotations"`
	Image         string            `json:"image"`
	Repo          string            `json:"repo"`
}

// metadataDir returns the directory where codespace metadata files are stored.
func metadataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".scion", "codespaces")
}

func metadataPath(codespaceName string) string {
	return filepath.Join(metadataDir(), codespaceName+".json")
}

func saveCodespaceMetadata(m codespaceMetadata) error {
	dir := metadataDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath(m.CodespaceName), data, 0644)
}

func loadCodespaceMetadata(codespaceName string) (codespaceMetadata, error) {
	var m codespaceMetadata
	data, err := os.ReadFile(metadataPath(codespaceName))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

func loadAllCodespaceMetadata() map[string]codespaceMetadata {
	result := make(map[string]codespaceMetadata)
	dir := metadataDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if m, err := loadCodespaceMetadata(name); err == nil {
			result[name] = m
		}
	}
	return result
}

func deleteCodespaceMetadata(codespaceName string) {
	_ = os.Remove(metadataPath(codespaceName))
}

// resolveRepo determines the GitHub owner/repo for codespace creation.
// It checks (in order): explicit config, RunConfig labels, git remote of the workspace.
func (r *CodespaceRuntime) resolveRepo(config RunConfig) string {
	if r.Repo != "" {
		return r.Repo
	}

	// Try to extract from the workspace or repo root git remote
	dir := config.RepoRoot
	if dir == "" {
		dir = config.Workspace
	}
	if dir == "" {
		return ""
	}

	remote := util.GetGitRemoteDir(dir)
	if remote == "" {
		return ""
	}
	return extractOwnerRepoFromRemote(remote)
}

// extractOwnerRepoFromRemote extracts "owner/repo" from a git remote URL.
func extractOwnerRepoFromRemote(remote string) string {
	remote = strings.TrimSpace(remote)

	// Handle SSH format: git@github.com:owner/repo.git
	if strings.Contains(remote, ":") && strings.Contains(remote, "@") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) == 2 {
			path := strings.TrimSuffix(parts[1], ".git")
			path = strings.TrimPrefix(path, "/")
			if isOwnerRepo(path) {
				return path
			}
		}
	}

	// Handle HTTPS: https://github.com/owner/repo.git
	for _, prefix := range []string{"https://", "http://", "ssh://", "git://"} {
		remote = strings.TrimPrefix(remote, prefix)
	}

	// Strip user info (e.g., x-access-token:TOKEN@)
	if idx := strings.Index(remote, "@"); idx >= 0 {
		remote = remote[idx+1:]
	}

	remote = strings.TrimSuffix(remote, ".git")

	// Split host/owner/repo
	parts := strings.SplitN(remote, "/", 2)
	if len(parts) == 2 {
		ownerRepo := parts[1]
		if isOwnerRepo(ownerRepo) {
			return ownerRepo
		}
	}

	return ""
}

func isOwnerRepo(s string) bool {
	parts := strings.Split(s, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

// getCurrentBranch returns the current git branch of the workspace directory.
func getCurrentBranch(config RunConfig) string {
	dir := config.Workspace
	if dir == "" {
		dir = config.RepoRoot
	}
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (r *CodespaceRuntime) Run(ctx context.Context, config RunConfig) (string, error) {
	repo := r.resolveRepo(config)
	if repo == "" {
		return "", fmt.Errorf("could not determine GitHub repository; set repo in codespace runtime config or ensure a git remote exists")
	}

	// Build gh cs create args
	displayName := fmt.Sprintf("scion-%s", config.Name)
	if len(displayName) > 48 {
		displayName = displayName[:48]
	}

	args := []string{"cs", "create", "-R", repo, "-d", displayName}
	if r.Machine != "" {
		args = append(args, "-m", r.Machine)
	}
	if r.IdleTimeout != "" {
		args = append(args, "--idle-timeout", r.IdleTimeout)
	}
	if r.RetentionPeriod != "" {
		args = append(args, "--retention-period", r.RetentionPeriod)
	}
	if r.DevcontainerPath != "" {
		args = append(args, "--devcontainer-path", r.DevcontainerPath)
	}
	if branch := getCurrentBranch(config); branch != "" {
		args = append(args, "-b", branch)
	}

	// Create codespace. gh cs create may emit progress lines before the name.
	out, err := runSimpleCommand(ctx, r.Command, args...)
	if err != nil {
		return "", fmt.Errorf("failed to create codespace: %w (output: %s)", err, out)
	}
	codespaceName := lastNonEmptyLine(out)
	if codespaceName == "" {
		return "", fmt.Errorf("gh cs create returned empty codespace name (full output: %s)", out)
	}

	// Persist metadata immediately so List() can track this codespace.
	if err := saveCodespaceMetadata(codespaceMetadata{
		CodespaceName: codespaceName,
		Labels:        config.Labels,
		Annotations:   config.Annotations,
		Image:         config.Image,
		Repo:          repo,
	}); err != nil {
		util.Debugf("codespace: failed to save metadata: %v", err)
	}

	// Build the startup script content now (while we still have the config)
	// so the background goroutine doesn't need to hold references to the harness.
	startupScript, err := r.buildStartupScript(config)
	if err != nil {
		return "", err
	}

	// Extract the hub port from env vars for the reverse SSH tunnel.
	hubPort := ""
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 && (parts[0] == "SCION_HUB_ENDPOINT" || parts[0] == "SCION_HUB_URL") && hubPort == "" {
			if u, err := url.Parse(parts[1]); err == nil {
				if p := u.Port(); p != "" {
					hubPort = p
				}
			}
		}
	}

	// Launch background provisioning: wait for codespace to be ready, then
	// copy the startup script and launch the harness via SSH. This allows
	// Run() to return immediately so the broker can respond to the hub
	// with a "provisioning" status instead of blocking for minutes.
	go r.provisionAsync(codespaceName, config.Name, startupScript, hubPort)

	return codespaceName, nil
}

// buildStartupScript constructs the bash script that sets up environment
// variables and starts the harness inside a tmux session.
func (r *CodespaceRuntime) buildStartupScript(config RunConfig) (string, error) {
	envLines := r.buildEnvScript(config)

	if config.Harness == nil {
		return "", fmt.Errorf("no harness provided")
	}
	harnessArgs := config.Harness.GetCommand(config.Task, config.Resume, config.CommandArgs)
	var quotedArgs []string
	for _, a := range harnessArgs {
		if strings.ContainsAny(a, " \t\n\"'$\\") {
			quotedArgs = append(quotedArgs, fmt.Sprintf("%q", a))
		} else {
			quotedArgs = append(quotedArgs, a)
		}
	}
	cmdLine := strings.Join(quotedArgs, " ")

	// Use separate tmux commands instead of chaining with \; because
	// the task argument may contain spaces/quotes that confuse tmux's
	// command separator parsing.
	tmuxCmd := fmt.Sprintf(
		"tmux new-session -d -s scion -n agent %s && tmux new-window -t scion -n shell && tmux select-window -t scion:agent",
		cmdLine,
	)

	// Derive the repo name for the codespace workspace path (/workspaces/<repo-name>).
	repoName := ""
	if repo := r.resolveRepo(config); repo != "" {
		parts := strings.Split(repo, "/")
		repoName = parts[len(parts)-1]
	}

	var script strings.Builder
	script.WriteString("#!/bin/bash\nset -e\n")
	// Ensure ~/.local/bin is on PATH — non-interactive SSH sessions may not
	// source the full shell profile, so tools installed there (e.g. claude)
	// would not be found.
	script.WriteString("export PATH=\"$HOME/.local/bin:$PATH\"\n")
	// Ensure tmux is available — codespace devcontainer images may not include it.
	script.WriteString("if ! command -v tmux &>/dev/null; then sudo apt-get update -qq && sudo apt-get install -y -qq tmux; fi\n")

	// cd into the workspace directory so the harness starts in the repo root.
	if repoName != "" {
		wsPath := fmt.Sprintf("/workspaces/%s", repoName)
		script.WriteString(fmt.Sprintf("cd %s\n", wsPath))

		// Pre-seed Claude Code config to skip onboarding and workspace trust prompts.
		script.WriteString(fmt.Sprintf(`if [ ! -f "$HOME/.claude.json" ]; then
  cat > "$HOME/.claude.json" << 'CLAUDE_EOF'
{"hasCompletedOnboarding":true,"projects":{"%s":{"hasTrustDialogAccepted":true}}}
CLAUDE_EOF
fi
`, wsPath))

	// Pre-seed Claude Code settings to skip the bypass-permissions confirmation.
	script.WriteString(`mkdir -p "$HOME/.claude"
if [ ! -f "$HOME/.claude/settings.json" ]; then
  echo '{"skipDangerousModePermissionPrompt":true}' > "$HOME/.claude/settings.json"
fi
`)
	}

	for _, line := range envLines {
		script.WriteString(line)
		script.WriteString("\n")
	}
	script.WriteString(tmuxCmd)
	script.WriteString("\n")
	return script.String(), nil
}

// provisionAsync runs in the background after Run() returns. It waits for the
// codespace to become available, starts a reverse SSH tunnel for hub
// connectivity, copies the startup script, and launches the harness. Errors
// are logged but cannot be returned to the caller since Run() has already
// returned the codespace name.
func (r *CodespaceRuntime) provisionAsync(codespaceName, agentName, startupScript, hubPort string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	runtimeLog.Info("Codespace provisioning started", "codespace", codespaceName, "agent", agentName)

	// 1. Wait for codespace to reach "Available" state
	if err := r.waitForReady(ctx, codespaceName); err != nil {
		runtimeLog.Error("Codespace did not become ready", "codespace", codespaceName, "error", err)
		return
	}

	// 2. Write startup script to a temp file and copy to codespace
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("scion-cs-start-%s.sh", agentName))
	if err := os.WriteFile(tmpFile, []byte(startupScript), 0755); err != nil {
		runtimeLog.Error("Failed to write startup script", "codespace", codespaceName, "error", err)
		return
	}
	defer os.Remove(tmpFile)

	// Use -e flag so scp expands ~ on the remote side
	if _, err := runSimpleCommand(ctx, r.Command, "cs", "cp", "-e", "-c", codespaceName, tmpFile, "remote:~/.scion-start.sh"); err != nil {
		runtimeLog.Error("Failed to copy startup script to codespace", "codespace", codespaceName, "error", err)
		return
	}

	// 3. Start reverse SSH tunnel so the codespace can reach the hub on localhost.
	// This must be started BEFORE the harness so hub connectivity is available
	// when the agent starts, but AFTER cp to avoid SSH multiplexing conflicts.
	if hubPort != "" {
		tunnelSpec := fmt.Sprintf("%s:localhost:%s", hubPort, hubPort)
		tunnelCmd := exec.Command(r.Command, "cs", "ssh", "-c", codespaceName, "--", "-N", "-R", tunnelSpec)
		if err := tunnelCmd.Start(); err != nil {
			runtimeLog.Error("Failed to start reverse SSH tunnel", "codespace", codespaceName, "port", hubPort, "error", err)
		} else {
			runtimeLog.Info("Reverse SSH tunnel started", "codespace", codespaceName, "port", hubPort, "pid", tunnelCmd.Process.Pid)
			// Reap the process when it exits to avoid zombies.
			go func() {
				if err := tunnelCmd.Wait(); err != nil {
					runtimeLog.Debug("Reverse SSH tunnel exited", "codespace", codespaceName, "error", err)
				}
			}()
		}
		// Brief pause to let the tunnel establish before starting the harness.
		time.Sleep(2 * time.Second)
	}

	// 4. Execute startup script (starts tmux session and returns)
	if _, err := runSimpleCommand(ctx, r.Command, "cs", "ssh", "-c", codespaceName, "--", "chmod +x ~/.scion-start.sh && ~/.scion-start.sh"); err != nil {
		runtimeLog.Error("Failed to start harness in codespace", "codespace", codespaceName, "error", err)
		return
	}

	runtimeLog.Info("Codespace provisioning completed", "codespace", codespaceName, "agent", agentName)
}

// codespaceViewEntry represents the JSON output from gh cs view.
type codespaceViewEntry struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// waitForReady polls the codespace state until it reaches "Available" or the
// context is cancelled. Codespaces transition through "Starting" → "Available".
func (r *CodespaceRuntime) waitForReady(ctx context.Context, codespaceName string) error {
	const (
		pollInterval = 5 * time.Second
		timeout      = 10 * time.Minute
	)

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %v waiting for codespace to become available", timeout)
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		out, err := runSimpleCommand(ctx, r.Command, "cs", "view", "-c", codespaceName, "--json", "name,state")
		if err != nil {
			runtimeLog.Debug("waitForReady: view failed, retrying", "codespace", codespaceName, "error", err)
			time.Sleep(pollInterval)
			continue
		}

		var entry codespaceViewEntry
		if err := json.Unmarshal([]byte(out), &entry); err != nil {
			runtimeLog.Debug("waitForReady: failed to parse view output", "output", out, "error", err)
			time.Sleep(pollInterval)
			continue
		}

		switch entry.State {
		case "Available":
			return nil
		case "Shutdown", "ShuttingDown", "Failed":
			return fmt.Errorf("codespace reached terminal state %q", entry.State)
		default:
			// Starting, Provisioning, etc. — keep polling
			runtimeLog.Debug("waitForReady: codespace not ready yet", "codespace", codespaceName, "state", entry.State)
			time.Sleep(pollInterval)
		}
	}
}

// codespaceSkipEnvVars lists environment variables that should not be injected
// into codespaces because the codespace provides its own values.
var codespaceSkipEnvVars = map[string]bool{
	"GITHUB_TOKEN": true,
}

// codespaceRewriteEnv rewrites env values for the codespace environment.
// Docker bridge hostnames (host.docker.internal) are replaced with localhost
// since codespaces use a reverse SSH tunnel to reach the host.
func codespaceRewriteEnv(v string) string {
	return strings.ReplaceAll(v, "host.docker.internal", "localhost")
}

// buildEnvScript collects environment variables from the RunConfig and returns
// export statements suitable for a bash script.
func (r *CodespaceRuntime) buildEnvScript(config RunConfig) []string {
	var lines []string

	addEnv := func(k, v string) {
		if codespaceSkipEnvVars[k] {
			return
		}
		lines = append(lines, fmt.Sprintf("export %s=%s", k, shellQuote(codespaceRewriteEnv(v))))
	}

	if config.Harness != nil {
		for k, v := range config.Harness.GetEnv(config.Name, config.HomeDir, config.UnixUsername) {
			addEnv(k, v)
		}
		if config.TelemetryEnabled {
			for k, v := range config.Harness.GetTelemetryEnv() {
				addEnv(k, v)
			}
		}
	}

	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			addEnv(parts[0], parts[1])
		}
	}

	for _, s := range config.ResolvedSecrets {
		if s.Type == "environment" || s.Type == "" {
			addEnv(s.Target, s.Value)
		}
	}

	return lines
}

// lastNonEmptyLine returns the last non-empty line from multi-line output.
// gh CLI commands often print progress/status lines before the actual result.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// shellQuote wraps a value in single quotes for safe shell expansion.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// resolveCodespaceName maps an id (which may be an agent name/slug or a
// codespace name) to the actual codespace name. It first checks if id is
// already a known codespace name via local metadata, then falls back to
// searching by agent name in the display-name convention "scion-<name>".
func (r *CodespaceRuntime) resolveCodespaceName(ctx context.Context, id string) (string, error) {
	// If local metadata exists for this id, it's already a codespace name.
	if _, err := loadCodespaceMetadata(id); err == nil {
		return id, nil
	}

	// Otherwise, search by listing codespaces and matching by agent name.
	agents, err := r.List(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list codespaces to resolve %q: %w", id, err)
	}
	for _, a := range agents {
		if a.Name == id || a.ContainerID == id {
			return a.ContainerID, nil
		}
	}
	return "", fmt.Errorf("no codespace found for agent %q", id)
}

func (r *CodespaceRuntime) Stop(ctx context.Context, id string) error {
	csName, err := r.resolveCodespaceName(ctx, id)
	if err != nil {
		return err
	}
	out, err := runSimpleCommand(ctx, r.Command, "cs", "stop", "-c", csName)
	if err != nil {
		return fmt.Errorf("failed to stop codespace: %w (output: %s)", err, out)
	}
	return nil
}

func (r *CodespaceRuntime) Delete(ctx context.Context, id string) error {
	csName, err := r.resolveCodespaceName(ctx, id)
	if err != nil {
		return err
	}
	out, err := runSimpleCommand(ctx, r.Command, "cs", "delete", "-c", csName, "-f")
	if err != nil {
		return fmt.Errorf("failed to delete codespace: %w (output: %s)", err, out)
	}
	deleteCodespaceMetadata(csName)
	return nil
}

// codespaceListEntry represents a single entry from gh cs list --json output.
type codespaceListEntry struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	State       string `json:"state"`
	Repository  string `json:"repository"`
	MachineName string `json:"machineName"`
	Owner       string `json:"owner"`
}

func (r *CodespaceRuntime) List(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
	out, err := runSimpleCommand(ctx, r.Command, "cs", "list", "--json", "name,displayName,state,repository,machineName,owner", "-L", "100")
	if err != nil {
		return nil, fmt.Errorf("failed to list codespaces: %w", err)
	}

	var entries []codespaceListEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, fmt.Errorf("failed to parse codespace list: %w", err)
	}

	allMeta := loadAllCodespaceMetadata()

	var agents []api.AgentInfo
	for _, e := range entries {
		// Only include scion-managed codespaces
		if !strings.HasPrefix(e.DisplayName, "scion-") {
			continue
		}

		labels := map[string]string{
			"scion.agent": "true",
		}
		annotations := map[string]string{}
		var image, template, harnessConfig, harnessAuth, grove, groveID, grovePath string

		if meta, ok := allMeta[e.Name]; ok {
			for k, v := range meta.Labels {
				labels[k] = v
			}
			for k, v := range meta.Annotations {
				annotations[k] = v
			}
			image = meta.Image
			template = labels["scion.template"]
			harnessConfig = labels["scion.harness_config"]
			harnessAuth = labels["scion.harness_auth"]
			grove = labels["scion.grove"]
			groveID = labels["scion.grove_id"]
			grovePath = annotations["scion.grove_path"]
		}

		agentName := labels["scion.name"]
		if agentName == "" {
			agentName = strings.TrimPrefix(e.DisplayName, "scion-")
		}

		// Filter by labels
		match := true
		for k, v := range labelFilter {
			if labels[k] != v && annotations[k] != v {
				match = false
				break
			}
		}
		if !match {
			continue
		}

		agents = append(agents, api.AgentInfo{
			ContainerID:     e.Name,
			Name:            agentName,
			ContainerStatus: e.State,
			Phase:           "created",
			Image:           image,
			Labels:          labels,
			Annotations:     annotations,
			Template:        template,
			HarnessConfig:   harnessConfig,
			HarnessAuth:     harnessAuth,
			Grove:           grove,
			GroveID:         groveID,
			GrovePath:       grovePath,
			Runtime:         r.Name(),
		})
	}

	return agents, nil
}

func (r *CodespaceRuntime) GetLogs(ctx context.Context, id string) (string, error) {
	csName, err := r.resolveCodespaceName(ctx, id)
	if err != nil {
		return "", err
	}
	return runSimpleCommand(ctx, r.Command, "cs", "logs", "-c", csName)
}

func (r *CodespaceRuntime) Attach(ctx context.Context, id string) error {
	csName, err := r.resolveCodespaceName(context.Background(), id)
	if err != nil {
		return err
	}
	return runInteractiveCommand(r.Command, "cs", "ssh", "-c", csName, "--", "tmux", "attach", "-t", "scion")
}

// ImageExists always returns true for codespaces since they use devcontainer
// configurations rather than pre-built container images.
func (r *CodespaceRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	return true, nil
}

// PullImage is a no-op for codespaces.
func (r *CodespaceRuntime) PullImage(ctx context.Context, image string) error {
	return nil
}

// Sync copies files between the local workspace and the codespace using gh cs cp.
func (r *CodespaceRuntime) Sync(ctx context.Context, id string, direction SyncDirection) error {
	csName, err := r.resolveCodespaceName(ctx, id)
	if err != nil {
		return err
	}
	meta, err := loadCodespaceMetadata(csName)
	if err != nil {
		return fmt.Errorf("failed to load codespace metadata: %w", err)
	}

	grovePath := meta.Annotations["scion.grove_path"]
	if grovePath == "" {
		return fmt.Errorf("codespace %s has no grove_path in metadata", id)
	}

	agentName := meta.Labels["scion.name"]
	if agentName == "" {
		return fmt.Errorf("codespace %s has no agent name in metadata", id)
	}

	// Determine local workspace path from the worktree pattern
	groveName := meta.Labels["scion.grove"]
	localWorkspace := filepath.Join(filepath.Dir(grovePath), ".scion_worktrees", groveName, agentName)

	// Determine the repo name for the codespace workspace path
	repo := meta.Repo
	parts := strings.Split(repo, "/")
	repoName := parts[len(parts)-1]
	remoteWorkspace := fmt.Sprintf("/workspaces/%s", repoName)

	switch direction {
	case SyncTo:
		_, err := runSimpleCommand(ctx, r.Command, "cs", "cp", "-c", csName, "-r", localWorkspace+"/.", fmt.Sprintf("remote:%s/", remoteWorkspace))
		return err
	case SyncFrom:
		_, err := runSimpleCommand(ctx, r.Command, "cs", "cp", "-c", csName, "-r", fmt.Sprintf("remote:%s/.", remoteWorkspace), localWorkspace+"/")
		return err
	default:
		return fmt.Errorf("sync direction must be specified for codespace runtime")
	}
}

func (r *CodespaceRuntime) Exec(ctx context.Context, id string, cmd []string) (string, error) {
	csName, err := r.resolveCodespaceName(ctx, id)
	if err != nil {
		return "", err
	}
	args := append([]string{"cs", "ssh", "-c", csName, "--"}, cmd...)
	return runSimpleCommand(ctx, r.Command, args...)
}

// GetWorkspacePath returns the local workspace path for the codespace agent.
// Since codespaces are remote, this returns the path from local metadata
// where synced files would be found.
func (r *CodespaceRuntime) GetWorkspacePath(ctx context.Context, id string) (string, error) {
	csName, err := r.resolveCodespaceName(ctx, id)
	if err != nil {
		return "", err
	}
	meta, err := loadCodespaceMetadata(csName)
	if err != nil {
		return "", fmt.Errorf("failed to load codespace metadata for %s: %w", csName, err)
	}

	grovePath := meta.Annotations["scion.grove_path"]
	agentName := meta.Labels["scion.name"]
	groveName := meta.Labels["scion.grove"]
	if grovePath == "" || agentName == "" || groveName == "" {
		return "", fmt.Errorf("incomplete metadata for codespace %s", id)
	}

	return filepath.Join(filepath.Dir(grovePath), ".scion_worktrees", groveName, agentName), nil
}
