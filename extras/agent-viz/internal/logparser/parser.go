package logparser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GCPLogEntry represents a single log entry from Google Cloud Logging export.
type GCPLogEntry struct {
	InsertID    string            `json:"insertId"`
	JSONPayload map[string]any    `json:"jsonPayload"`
	Timestamp   string            `json:"timestamp"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels"`
	LogName     string            `json:"logName"`
}

// PlaybackManifest is sent once at connection start.
type PlaybackManifest struct {
	Type      string      `json:"type"`
	TimeRange TimeRange   `json:"timeRange"`
	Agents    []AgentInfo `json:"agents"`
	Files     []FileNode  `json:"files"`
	GroveID   string      `json:"groveId"`
	GroveName string      `json:"groveName"`
}

type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type AgentInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
	Color   string `json:"color"`
}

type FileNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Parent string `json:"parent"`
	IsDir  bool   `json:"isDir"`
}

// PlaybackEvent is streamed during playback.
type PlaybackEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      any    `json:"data"`
}

type AgentStateEvent struct {
	AgentID  string `json:"agentId"`
	Phase    string `json:"phase,omitempty"`
	Activity string `json:"activity,omitempty"`
	ToolName string `json:"toolName,omitempty"`
}

type MessageEvent struct {
	Sender      string `json:"sender"`
	Recipient   string `json:"recipient"`
	MsgType     string `json:"msgType"`
	Content     string `json:"content,omitempty"`
	Broadcasted bool   `json:"broadcasted"`
}

type FileEditEvent struct {
	AgentID  string `json:"agentId"`
	FilePath string `json:"filePath"`
	Action   string `json:"action"`
}

type AgentLifecycleEvent struct {
	AgentID string `json:"agentId"`
	Name    string `json:"name"`
	Action  string `json:"action"`
}

// Agent colors assigned in order.
var agentColors = []string{
	"#4e79a7", "#f28e2b", "#e15759", "#76b7b2",
	"#59a14f", "#edc948", "#b07aa1", "#ff9da7",
	"#9c755f", "#bab0ac",
}

// ParseResult contains all parsed data from log files.
type ParseResult struct {
	Manifest PlaybackManifest
	Events   []PlaybackEvent
}

// ParseLogFile reads a GCP log JSON export and returns the manifest and events.
func ParseLogFile(path string) (*ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading log file: %w", err)
	}

	var entries []GCPLogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing log JSON: %w", err)
	}

	// Sort entries by timestamp
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})

	agents := extractAgents(entries)
	files := extractFiles(entries)
	events := extractEvents(entries, agents)
	timeRange := extractTimeRange(entries)

	// Determine grove info
	groveID, groveName := extractGroveInfo(entries)

	manifest := PlaybackManifest{
		Type:      "manifest",
		TimeRange: timeRange,
		Agents:    agents,
		Files:     files,
		GroveID:   groveID,
		GroveName: groveName,
	}

	return &ParseResult{
		Manifest: manifest,
		Events:   events,
	}, nil
}

func extractTimeRange(entries []GCPLogEntry) TimeRange {
	if len(entries) == 0 {
		return TimeRange{}
	}
	return TimeRange{
		Start: entries[0].Timestamp,
		End:   entries[len(entries)-1].Timestamp,
	}
}

func extractGroveInfo(entries []GCPLogEntry) (string, string) {
	for _, e := range entries {
		if gid, ok := e.Labels["grove_id"]; ok {
			// Try to find grove name from server logs
			name := gid
			if slug, ok := e.JSONPayload["slug"]; ok {
				if s, ok := slug.(string); ok && !strings.Contains(s, ":") {
					name = s
				}
			}
			return gid, name
		}
	}
	return "", ""
}

func extractAgents(entries []GCPLogEntry) []AgentInfo {
	agentMap := make(map[string]*AgentInfo)
	nameMap := make(map[string]string) // id -> name
	// Track which IDs had explicit lifecycle events (pre_start)
	hasLifecycle := make(map[string]bool)

	// First pass: find agent names and IDs from message events.
	// Messages reference agents by slug name (sender/recipient) and UUID (sender_id/recipient_id).
	for _, e := range entries {
		logName := logBaseName(e.LogName)
		if logName == "scion-messages" {
			jp := e.JSONPayload
			for _, field := range []string{"sender", "recipient"} {
				val := getStr(jp, field)
				if val == "" {
					val = e.Labels[field]
				}
				idField := field + "_id"
				aid := getStr(jp, idField)
				if aid == "" {
					aid = e.Labels[idField]
				}
				if strings.HasPrefix(val, "agent:") {
					name := strings.TrimPrefix(val, "agent:")
					if aid != "" {
						nameMap[aid] = name
					} else {
						// No UUID available — use the slug name as both ID and name
						nameMap[name] = name
					}
				}
			}
			// Also check agent_name / agent_id fields in message payloads
			if an := getStr(jp, "agent_name"); an != "" {
				if aid := getStr(jp, "agent_id"); aid != "" {
					nameMap[aid] = an
				}
			}
		}
	}

	// Second pass: collect agents from scion-agents logs (these have UUIDs and harness info)
	for _, e := range entries {
		logName := logBaseName(e.LogName)
		if logName != "scion-agents" {
			continue
		}
		aid := e.Labels["agent_id"]
		if aid == "" {
			continue
		}
		if _, exists := agentMap[aid]; !exists {
			harness := e.Labels["scion.harness"]
			name := nameMap[aid]
			if name == "" && len(aid) >= 8 {
				name = aid[:8]
			} else if name == "" {
				name = aid
			}
			agentMap[aid] = &AgentInfo{
				ID:      aid,
				Name:    name,
				Harness: harness,
			}
		}
		msg := getStr(e.JSONPayload, "message")
		if msg == "agent.lifecycle.pre_start" {
			hasLifecycle[aid] = true
		}
	}

	// Third pass: backfill agents discovered only from messages (no scion-agents entries).
	// These are agents that existed before the log window or whose agent logs weren't captured.
	for id, name := range nameMap {
		if _, exists := agentMap[id]; !exists {
			agentMap[id] = &AgentInfo{
				ID:      id,
				Name:    name,
				Harness: "unknown",
			}
		}
	}

	// Assign colors after sorting for deterministic output
	agents := make([]AgentInfo, 0, len(agentMap))
	for _, a := range agentMap {
		agents = append(agents, *a)
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	for i := range agents {
		agents[i].Color = agentColors[i%len(agentColors)]
	}

	return agents
}

// agentsWithLifecycle returns the set of agent IDs that had explicit lifecycle events.
func agentsWithLifecycle(entries []GCPLogEntry) map[string]bool {
	has := make(map[string]bool)
	for _, e := range entries {
		if logBaseName(e.LogName) != "scion-agents" {
			continue
		}
		msg := getStr(e.JSONPayload, "message")
		if msg == "agent.lifecycle.pre_start" {
			has[e.Labels["agent_id"]] = true
		}
	}
	return has
}

func extractFiles(entries []GCPLogEntry) []FileNode {
	filePaths := make(map[string]bool)

	for _, e := range entries {
		logName := logBaseName(e.LogName)
		if logName != "scion-agents" {
			continue
		}
		jp := e.JSONPayload
		toolName := getStr(jp, "tool_name")
		if !isFileEditTool(toolName) {
			continue
		}
		fp := extractFilePath(jp)
		if fp != "" {
			filePaths[fp] = true
		}
	}

	// Build file tree nodes from discovered paths (may be empty — that's fine)
	nodes := make(map[string]*FileNode)
	for fp := range filePaths {
		addFileToTree(nodes, fp)
	}

	result := make([]FileNode, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, *n)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// extractFilePath tries to find a file path from a tool call's JSON payload.
func extractFilePath(jp map[string]any) string {
	for _, key := range []string{"file_path", "path", "filePath", "filename"} {
		fp := getStr(jp, key)
		if fp != "" {
			return normalizeFilePath(fp)
		}
	}
	return ""
}

// normalizeFilePath strips workspace prefixes and relative path markers.
func normalizeFilePath(fp string) string {
	fp = strings.TrimPrefix(fp, "/workspace/")
	fp = strings.TrimPrefix(fp, "./")
	return fp
}

func addFileToTree(nodes map[string]*FileNode, fp string) {
	// Add file node
	if _, exists := nodes[fp]; !exists {
		nodes[fp] = &FileNode{
			ID:     fp,
			Name:   filepath.Base(fp),
			Parent: filepath.Dir(fp),
			IsDir:  false,
		}
	}

	// Add parent directories
	dir := filepath.Dir(fp)
	for dir != "." && dir != "" {
		if _, exists := nodes[dir]; !exists {
			nodes[dir] = &FileNode{
				ID:     dir,
				Name:   filepath.Base(dir),
				Parent: filepath.Dir(dir),
				IsDir:  true,
			}
		}
		dir = filepath.Dir(dir)
	}
}

func extractEvents(entries []GCPLogEntry, agents []AgentInfo) []PlaybackEvent {
	var events []PlaybackEvent

	agentNameByID := make(map[string]string)
	for _, a := range agents {
		agentNameByID[a.ID] = a.Name
	}

	// Track which agents had explicit lifecycle (pre_start) events
	hasLifecycle := agentsWithLifecycle(entries)
	// Track which agents we've already emitted a backfill create event for
	backfilled := make(map[string]bool)

	// Helper: ensure an agent has a create event. For agents without explicit lifecycle
	// events, we emit a synthetic agent_create at the timestamp of their first appearance.
	ensureAgent := func(agentID, ts string) {
		if hasLifecycle[agentID] || backfilled[agentID] {
			return
		}
		backfilled[agentID] = true
		events = append(events, PlaybackEvent{
			Type:      "agent_create",
			Timestamp: ts,
			Data: AgentLifecycleEvent{
				AgentID: agentID,
				Name:    agentNameByID[agentID],
				Action:  "create",
			},
		})
		events = append(events, PlaybackEvent{
			Type:      "agent_state",
			Timestamp: ts,
			Data: AgentStateEvent{
				AgentID:  agentID,
				Phase:    "running",
				Activity: "idle",
			},
		})
	}

	for _, e := range entries {
		logName := logBaseName(e.LogName)
		jp := e.JSONPayload
		ts := e.Timestamp

		switch logName {
		case "scion-agents":
			msg := getStr(jp, "message")
			aid := e.Labels["agent_id"]

			// Backfill agent if first appearance and no lifecycle event
			if aid != "" {
				ensureAgent(aid, ts)
			}

			switch msg {
			case "agent.session.start":
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID:  aid,
						Phase:    "running",
						Activity: "idle",
					},
				})
			case "agent.session.end":
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID:  aid,
						Phase:    "stopped",
						Activity: "completed",
					},
				})
			case "agent.turn.start":
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID:  aid,
						Activity: "thinking",
					},
				})
			case "agent.turn.end":
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID:  aid,
						Activity: "idle",
					},
				})
			case "agent.tool.call":
				toolName := getStr(jp, "tool_name")
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID:  aid,
						Activity: "executing",
						ToolName: toolName,
					},
				})
				// Generate file edit event for file-modifying tools
				if isFileEditTool(toolName) {
					fp := extractFilePath(jp)
					if fp != "" {
						action := "edit"
						if toolName == "write_file" || toolName == "create_file" || toolName == "Write" {
							action = "create"
						}
						events = append(events, PlaybackEvent{
							Type:      "file_edit",
							Timestamp: ts,
							Data: FileEditEvent{
								AgentID:  aid,
								FilePath: fp,
								Action:   action,
							},
						})
					}
				}
			case "agent.tool.result":
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID:  aid,
						Activity: "thinking",
					},
				})
			case "agent.lifecycle.pre_start":
				events = append(events, PlaybackEvent{
					Type:      "agent_create",
					Timestamp: ts,
					Data: AgentLifecycleEvent{
						AgentID: aid,
						Name:    agentNameByID[aid],
						Action:  "create",
					},
				})
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID: aid,
						Phase:   "starting",
					},
				})
			case "agent.lifecycle.pre_stop":
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID: aid,
						Phase:   "stopping",
					},
				})
			case "Tool requires confirmation":
				events = append(events, PlaybackEvent{
					Type:      "agent_state",
					Timestamp: ts,
					Data: AgentStateEvent{
						AgentID:  aid,
						Activity: "waiting_for_input",
					},
				})
			}

		case "scion-messages":
			// Skip "accepted" duplicates — only process dispatched messages
			msgAction := getStr(jp, "message")
			if strings.Contains(msgAction, "accepted") {
				continue
			}

			sender := getStr(jp, "sender")
			if sender == "" {
				sender = e.Labels["sender"]
			}
			recipient := getStr(jp, "recipient")
			if recipient == "" {
				recipient = e.Labels["recipient"]
			}
			msgType := getStr(jp, "msg_type")
			if msgType == "" {
				msgType = e.Labels["msg_type"]
			}
			content := getStr(jp, "message_content")
			broadcasted := getBool(jp, "broadcasted")

			if sender != "" && recipient != "" {
				senderName := strings.TrimPrefix(sender, "agent:")
				recipientName := strings.TrimPrefix(recipient, "agent:")

				// Backfill agents referenced in messages
				senderID := getStr(jp, "sender_id")
				if senderID == "" {
					senderID = e.Labels["sender_id"]
				}
				if senderID == "" && strings.HasPrefix(sender, "agent:") {
					senderID = senderName
				}
				recipientID := getStr(jp, "recipient_id")
				if recipientID == "" {
					recipientID = e.Labels["recipient_id"]
				}
				if recipientID == "" && strings.HasPrefix(recipient, "agent:") {
					recipientID = recipientName
				}

				if senderID != "" && strings.HasPrefix(sender, "agent:") {
					ensureAgent(senderID, ts)
				}
				if recipientID != "" && strings.HasPrefix(recipient, "agent:") {
					ensureAgent(recipientID, ts)
				}

				// Resolve UUID-based names to friendly names
				// e.g., "agent:a35ea791-..." should become "orchestrator"
				if agentNameByID[senderName] != "" {
					senderName = agentNameByID[senderName]
				} else if senderID != "" && agentNameByID[senderID] != "" {
					senderName = agentNameByID[senderID]
				}
				if agentNameByID[recipientName] != "" {
					recipientName = agentNameByID[recipientName]
				} else if recipientID != "" && agentNameByID[recipientID] != "" {
					recipientName = agentNameByID[recipientID]
				}

				events = append(events, PlaybackEvent{
					Type:      "message",
					Timestamp: ts,
					Data: MessageEvent{
						Sender:      senderName,
						Recipient:   recipientName,
						MsgType:     msgType,
						Content:     content,
						Broadcasted: broadcasted,
					},
				})
			}
		}
	}

	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp < events[j].Timestamp
	})

	return events
}

func isFileEditTool(name string) bool {
	switch name {
	case "write_file", "create_file", "Write", "edit_file", "Edit", "patch_file":
		return true
	}
	return false
}

func logBaseName(logName string) string {
	parts := strings.Split(logName, "/")
	return parts[len(parts)-1]
}

func getStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// TimestampToTime parses an ISO 8601 timestamp.
func TimestampToTime(ts string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, ts)
}
