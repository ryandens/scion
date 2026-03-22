import { WSClient } from './ws';
import { FileGraph } from './graph';
import { AgentRing } from './agents';
import { MessageRenderer } from './messages';
import { FileEditRenderer } from './files';
import { PlaybackControls } from './playback';
import type {
  PlaybackManifest,
  PlaybackEvent,
  StatusUpdate,
  AgentStateEvent,
  MessageEvent,
  FileEditEvent,
  AgentLifecycleEvent,
} from './types';

// Main application state
let fileGraph: FileGraph;
let agentRing: AgentRing;
let messageRenderer: MessageRenderer;
let fileEditRenderer: FileEditRenderer;
let playbackControls: PlaybackControls;
let overlayCanvas: HTMLCanvasElement;
let overlayCtx: CanvasRenderingContext2D;
let animFrameId: number;
let manifest: PlaybackManifest | null = null;

function init(): void {
  const graphContainer = document.getElementById('graph-container')!;
  const controlsContainer = document.getElementById('controls-container')!;

  // Create overlay canvas for agents, messages, particles
  overlayCanvas = document.createElement('canvas');
  overlayCanvas.id = 'overlay-canvas';
  overlayCanvas.style.cssText =
    'position:absolute;top:0;left:0;width:100%;height:100%;pointer-events:none;z-index:10;';
  graphContainer.appendChild(overlayCanvas);
  overlayCtx = overlayCanvas.getContext('2d')!;

  // Initialize components
  fileGraph = new FileGraph(graphContainer);
  agentRing = new AgentRing();
  messageRenderer = new MessageRenderer();
  fileEditRenderer = new FileEditRenderer();

  // WebSocket
  const ws = new WSClient();
  playbackControls = new PlaybackControls(controlsContainer, ws);

  ws.onMessage((msg) => {
    if ('type' in msg) {
      switch (msg.type) {
        case 'manifest':
          handleManifest(msg as PlaybackManifest);
          break;
        case 'status':
          playbackControls.updateStatus(msg as StatusUpdate);
          break;
        case 'agent_state':
        case 'message':
        case 'file_edit':
        case 'agent_create':
        case 'agent_destroy':
          handleEvent(msg as PlaybackEvent);
          break;
      }
    }
  });

  // Handle resize
  window.addEventListener('resize', handleResize);
  handleResize();

  // Connect
  ws.connect();

  // Start animation loop
  animate();
}

function handleManifest(m: PlaybackManifest): void {
  manifest = m;
  console.log('[agent-viz] Manifest received:', {
    agents: m.agents.length,
    files: m.files.length,
    timeRange: m.timeRange,
    groveId: m.groveId,
    groveName: m.groveName,
  });
  console.log('[agent-viz] Agents:', m.agents.map((a) => `${a.name} (${a.id.substring(0, 8)})`));

  // Update title
  const title = document.getElementById('grove-title');
  if (title) {
    title.textContent = m.groveName || m.groveId || 'Agent Visualizer';
  }

  // Initialize file graph
  fileGraph.init(m.files);

  // Initialize agent ring
  const w = overlayCanvas.width;
  const h = overlayCanvas.height;
  console.log('[agent-viz] Canvas dimensions:', w, 'x', h);
  agentRing.init(m.agents, w / 2, h / 2);

  // Set up playback controls
  playbackControls.setTimeRange(m.timeRange.start, m.timeRange.end);
  playbackControls.setAgents(m.agents);

  // Update info display
  const info = document.getElementById('info-display');
  if (info) {
    info.textContent = `${m.agents.length} agents | ${m.files.length} files`;
  }
}

function handleEvent(evt: PlaybackEvent): void {
  console.log('[agent-viz] Event:', evt.type, evt.data);
  switch (evt.type) {
    case 'agent_state':
      agentRing.updateState(evt.data as AgentStateEvent);
      break;
    case 'message':
      messageRenderer.addMessage(evt.data as MessageEvent, agentRing);
      break;
    case 'file_edit': {
      const fileEvt = evt.data as FileEditEvent;
      // Dynamically add file to graph if not already present
      if (fileEvt.filePath && !fileGraph.hasFile(fileEvt.filePath)) {
        fileGraph.addFile(fileEvt.filePath);
      }
      fileEditRenderer.addFileEdit(fileEvt, agentRing, fileGraph);
      break;
    }
    case 'agent_create': {
      const lifecycle = evt.data as AgentLifecycleEvent;
      // Dynamically add agent to the ring if not already present
      const agentInManifest = manifest?.agents.find(
        (a) => a.id === lifecycle.agentId || a.name === lifecycle.name
      );
      if (agentInManifest) {
        agentRing.addAgent(agentInManifest);
      } else {
        // Agent not in manifest — create an ad-hoc entry
        agentRing.addAgent({
          id: lifecycle.agentId,
          name: lifecycle.name || lifecycle.agentId.substring(0, 8),
          harness: 'unknown',
          color: '#888',
        });
      }
      agentRing.updateState({
        agentId: lifecycle.agentId,
        phase: 'created',
        activity: 'idle',
      });
      break;
    }
    case 'agent_destroy':
      agentRing.updateState({
        agentId: (evt.data as AgentLifecycleEvent).agentId,
        phase: 'stopped',
        activity: 'completed',
      });
      break;
  }
}

function handleResize(): void {
  const w = window.innerWidth;
  const h = window.innerHeight - 60; // reserve space for controls

  overlayCanvas.width = w;
  overlayCanvas.height = h;

  fileGraph.resize(w, h);
  agentRing.updateLayout(w / 2, h / 2);
}

function animate(): void {
  // Clear overlay
  overlayCtx.clearRect(0, 0, overlayCanvas.width, overlayCanvas.height);

  // Draw agents on ring
  agentRing.draw(overlayCtx);

  // Draw message lines
  messageRenderer.draw(overlayCtx);

  // Draw file edit particles
  fileEditRenderer.draw(overlayCtx);

  animFrameId = requestAnimationFrame(animate);
}

// Start when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
