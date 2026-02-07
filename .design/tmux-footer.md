# Design: Improved Tmux Footer for Scion Agents

## Overview
Improve the Scion tmux status bar to provide better context about the running agent, including the template name, agent name, and broker name.

## Current State
The default tmux status bar shows:
- Left: `[scion]` (Session name)
- Middle: `0:cmd*` (Window index and name)
- Right: `"pane title" HH:MM DD-Mon-YY`

Example:
`[scion] 0:[tmux]* "✳ Claude Code" 14:42 07-Feb-26`

## Proposed Changes

### 1. Environment Variables
Inject the following environment variables into the agent container's environment:
- `SCION_AGENT_NAME`: The name of the agent (e.g., `my-agent`).
- `SCION_TEMPLATE_NAME`: The name of the template used (e.g., `gemini-code`).
- `SCION_BROKER_NAME`: The name of the broker executing the agent (e.g., `local-docker` or `local`).

### 2. Tmux Configuration
Update `pkg/config/embeds/common/.tmux.conf` to customize the status bar.

#### Status Left
Keep it simple or refine to match Scion branding.
```tmux
set -g status-left "[scion] "
```

#### Status Right
Include the new environment variables with subtle color differences for better readability.
```tmux
set -g status-right "#[fg=colour244]#(echo $SCION_TEMPLATE_NAME) #[fg=colour136]/ #[fg=colour166]#(echo $SCION_AGENT_NAME) #[fg=colour136](#(echo $SCION_BROKER_NAME)) #[fg=colour136]%H:%M %d-%b-%y"
```

**Note**: Environment variables in tmux status bars must be accessed via shell commands using `#(echo $VAR)`. The `#{...}` syntax is for tmux internal format variables, not shell environment variables.

## Implementation Plan

### Phase 1: Environment Injection
- **pkg/agent/run.go**:
    - Update `Start` to include `SCION_AGENT_NAME` and `SCION_TEMPLATE_NAME` in the environment.
    - Default `SCION_BROKER_NAME` to `local` if not provided.
- **pkg/runtimebroker/handlers.go**:
    - Update `createAgent` to inject `SCION_BROKER_NAME` from server configuration into the environment.

### Phase 2: Configuration Update
- **pkg/config/embeds/common/.tmux.conf**:
    - Add `status-left` and `status-right` configurations.
    - Use `colour244` (Grey), `colour166` (Orange), and `colour136` (Yellow) for field separation.

### Phase 3: Verification (Manual)
Since the agent is running within a tmux session, automated verification of the visual footer is not feasible. The following manual steps should be performed by a human:
- Start an agent in solo mode and verify the footer shows `(local)`.
- Start an agent via a runtime broker and verify the broker name is correctly displayed.
- Verify that colors provide subtle but clear separation of fields.

## Consideration: Long Names
If names are too long, they might be truncated or push the clock off-screen. We should consider a maximum width or smart truncation if necessary.

Example with values:
`[scion] gemini-code / my-agent (local) 14:42 07-Feb-26`