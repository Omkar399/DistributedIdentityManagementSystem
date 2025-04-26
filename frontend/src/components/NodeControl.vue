<template>
  <div class="node-control-container">
    <h3>Node Control (Requires Docker Access)</h3>
    <!-- Add Display for Current Leader -->
    <div class="leader-display">
        Current Leader:
        <strong v-if="currentLeaderId > 0">Node {{ currentLeaderId }}</strong>
        <span v-else>Unknown / Election in Progress?</span>
    </div>
    <p class="warning">Warning: These actions directly interact with Docker containers.</p>
    <!-- Message area remains the same -->
    <div v-if="message" :class="['message', messageType]">{{ message }}</div>
    <table>
      <thead>
        <tr>
          <th>Node ID</th>
          <th>Status</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
         <!-- Add dynamic class binding for leader row -->
        <tr v-for="nodeId in nodeIds" :key="nodeId" :class="{ 'leader-row': nodeId === currentLeaderId }">
          <td>Node {{ nodeId }}</td>
          <td>
             <span :class="['status-indicator', getNodeRunStatusClass(nodeId)]">
               {{ getNodeRunStatusText(nodeId) }}
             </span>
          </td>
          <td>
            <button @click="controlNode(nodeId, 'start')" :disabled="loading[nodeId]">
              Start
            </button>
            <button @click="controlNode(nodeId, 'stop')" :disabled="loading[nodeId]" class="stop-button">
              Stop
            </button>
            <span v-if="loading[nodeId]" class="loading-indicator"> ...processing</span>
          </td>
        </tr>
      </tbody>
    </table>
     <div v-if="statusLoading" class="status-fetch-loading">Fetching node statuses...</div>
     <p v-if="statusFetchError" class="status-fetch-error">{{ statusFetchError }}</p>
  </div>
</template>

<script>
import axios from 'axios';

const CONTROL_API_URL_BASE = 'http://localhost:8090/control/node';
const STATUS_API_URL = 'http://localhost:8090/replication-summary';
// --- Add Leader API URL ---
const LEADER_API_URL = 'http://localhost:8090/current-leader';
// --- End ---
const STATUS_POLLING_INTERVAL = 5000; // 5 seconds for status
const LEADER_POLLING_INTERVAL = 3000; // Poll leader slightly faster (e.g., 3 seconds)


export default {
  name: 'NodeControl',
  data() {
    return {
      nodeIds: [1, 2, 3, 4],
      loading: {}, // For Start/Stop actions
      message: '',
      messageType: 'info',
      nodeRunStatus: {},
      statusLoading: false,
      statusFetchError: null,
      statusIntervalId: null,
      // --- Add leader data ---
      currentLeaderId: -1, // Initialize to -1 (unknown)
      leaderIntervalId: null,
      // --- End leader data ---
    };
  },
  methods: {
    // --- Keep run status methods (fetchRunStatus, startStatusPolling, stopStatusPolling, getNodeRunStatusText, getNodeRunStatusClass) ---
    async fetchRunStatus() {
      this.statusLoading = true;
      try {
        const response = await axios.get(STATUS_API_URL);
        this.nodeRunStatus = response.data.nodes || {};
        this.statusFetchError = null;
      } catch (err) {
        this.statusFetchError = `Failed to fetch node run statuses: ${err.message}`;
        console.error("Fetch Run Status Error:", err);
      } finally {
        this.statusLoading = false;
      }
    },
    startStatusPolling() {
      this.stopStatusPolling();
      this.fetchRunStatus();
      this.statusIntervalId = setInterval(this.fetchRunStatus, STATUS_POLLING_INTERVAL);
    },
    stopStatusPolling() {
      if (this.statusIntervalId) {
        clearInterval(this.statusIntervalId);
        this.statusIntervalId = null;
      }
    },
    getNodeRunStatusText(nodeId) {
        const status = this.nodeRunStatus[nodeId];
        if (status === undefined && !this.statusLoading) return 'Unknown';
        if (status?.error) return 'Stopped/Unreachable';
        if (status) return 'Running';
        return 'Checking...';
    },
    getNodeRunStatusClass(nodeId) {
        const status = this.nodeRunStatus[nodeId];
        if (status === undefined && !this.statusLoading) return 'unknown';
        if (status?.error) return 'stopped';
        if (status) return 'running';
        return 'unknown';
    },
    // --- End run status methods ---

    // --- Keep controlNode method ---
    async controlNode(nodeId, action) {
      this.loading[nodeId] = true;
      this.message = `Sending '${action}' request for Node ${nodeId}...`;
      this.messageType = 'info';

      try {
        const response = await axios.post(`${CONTROL_API_URL_BASE}/${nodeId}/${action}`);
        this.message = response.data.message || `Node ${nodeId} ${action} request successful.`;
        this.messageType = 'success';
        console.log(`Node ${nodeId} ${action} response:`, response.data);

        // Trigger immediate refresh of both statuses after sending command
        setTimeout(() => {
          this.fetchRunStatus();
          this.fetchCurrentLeader(); // Also fetch leader status
        }, 1500);

      } catch (err) {
        this.message = err.response?.data?.message || `Failed to ${action} Node ${nodeId}: ${err.message}`;
        this.messageType = 'error';
        console.error(`Error ${action} Node ${nodeId}:`, err.response || err);
      } finally {
        this.loading[nodeId] = false;
        setTimeout(() => { this.message = ''; }, 7000);
      }
    },
    // --- End controlNode method ---

    // --- Add methods for fetching and polling leader ---
    async fetchCurrentLeader() {
        try {
            const response = await axios.get(LEADER_API_URL);
            // Expecting { "currentLeaderId": X } where X might be -1
            this.currentLeaderId = response.data.currentLeaderId ?? -1;
        } catch (err) {
             console.error("Failed to fetch current leader:", err);
             // Optionally set an error state or keep last known leader
             // this.currentLeaderId = -1; // Reset on error?
        }
    },
    startLeaderPolling() {
        this.stopLeaderPolling();
        this.fetchCurrentLeader(); // Fetch immediately
        this.leaderIntervalId = setInterval(this.fetchCurrentLeader, LEADER_POLLING_INTERVAL);
    },
    stopLeaderPolling() {
         if (this.leaderIntervalId) {
             clearInterval(this.leaderIntervalId);
             this.leaderIntervalId = null;
         }
    }
    // --- End leader methods ---
  },
  created() {
    this.nodeIds.forEach(id => {
       this.loading[id] = false;
    });
  },
  mounted() {
      this.startStatusPolling();
      this.startLeaderPolling(); // Start polling for leader too
  },
  beforeUnmount() {
      this.stopStatusPolling();
      this.stopLeaderPolling(); // Stop polling for leader
  }
};
</script>

<style scoped>
/* ... Keep existing styles for container, warning, table, buttons, messages, status indicator ... */
.node-control-container { border: 1px solid #ccc; padding: 15px; margin-top: 20px; border-radius: 5px; background-color: #fdf5e6; }
h3 { margin-top: 0; }
.warning { color: #8a6d3b; background-color: #fcf8e3; border: 1px solid #faebcc; padding: 8px; border-radius: 4px; margin-bottom: 15px; font-size: 0.9em; }
table { width: 100%; border-collapse: collapse; margin-top: 10px;}
th, td { border: 1px solid #ddd; padding: 8px; text-align: left; vertical-align: middle;}
thead { background-color: #e0e0e0; }
button { margin-right: 5px; cursor: pointer; padding: 5px 10px;}
button:disabled { cursor: not-allowed; opacity: 0.6; }
.stop-button { background-color: #f44336; color: white; border: none;}
.stop-button:hover:not(:disabled) { background-color: #d32f2f; }
.stop-button:disabled { background-color: #ef9a9a;}
.start-button:disabled { background-color: #a5d6a7;}
.loading-indicator { font-style: italic; color: #555; margin-left: 10px; }
.message { padding: 10px; margin-bottom: 15px; border-radius: 4px; font-weight: bold; }
.message.info { background-color: #e7f3fe; color: #31708f; border: 1px solid #bce8f1;}
.message.success { background-color: #dff0d8; color: #3c763d; border: 1px solid #d6e9c6;}
.message.error { background-color: #f2dede; color: #a94442; border: 1px solid #ebccd1;}
.status-indicator { display: inline-block; padding: 3px 8px; border-radius: 10px; font-size: 0.85em; font-weight: bold; color: white; min-width: 70px; text-align: center; }
.status-indicator.running { background-color: #4CAF50; }
.status-indicator.stopped { background-color: #f44336; }
.status-indicator.unknown { background-color: #757575; }
.status-fetch-loading, .status-fetch-error { font-size: 0.8em; font-style: italic; margin-top: 10px; padding: 5px; }
.status-fetch-error { color: #a94442; }

/* --- Style for Leader Display --- */
.leader-display {
    font-size: 1.1em;
    margin-bottom: 15px;
    padding: 8px;
    background-color: #e3f2fd; /* Light blue background */
    border-left: 5px solid #2196F3; /* Blue accent border */
    border-radius: 4px;
}
.leader-display strong {
    color: #1565C0; /* Darker blue for emphasis */
}
/* --- Style for Highlighting Leader Row --- */
tr.leader-row td {
    background-color: #e3f2fd !important; /* Light blue background for leader row cells */
    font-weight: bold;
}
/* --- End Leader Styles --- */
</style>
