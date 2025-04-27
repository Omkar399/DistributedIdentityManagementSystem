<template>
  <div class="form-container">
    <h2>Node Control</h2>

    <div class="leader-display">
      Current Leader:
      <strong v-if="currentLeaderId > 0">Node {{ currentLeaderId }}</strong>
      <span v-else>Unknown / Election in Progress?</span>
    </div>

    <p class="warning-message">Warning: These actions directly interact with Docker containers.</p>

    <div v-if="message" :class="['alert', messageType]">
      <span class="alert-icon" v-if="messageType === 'success'">✓</span>
      <span class="alert-icon" v-if="messageType === 'error'">!</span>
      <span class="alert-icon" v-if="messageType === 'info'">i</span>
      {{ message }}
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>Node ID</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
           <tr v-for="nodeId in nodeIds" :key="nodeId" :class="{ 'leader-row': nodeId === currentLeaderId }">
            <td>Node {{ nodeId }}</td>
            <td>
              <span :class="['status-indicator', getNodeRunStatusClass(nodeId)]">
                {{ getNodeRunStatusText(nodeId) }}
              </span>
            </td>
            <td class="actions-cell">
              <button @click="controlNode(nodeId, 'start')" :disabled="loading[nodeId]" class="action-button start-button">
                Start
              </button>
              <button @click="controlNode(nodeId, 'stop')" :disabled="loading[nodeId]" class="action-button stop-button">
                Stop
              </button>
              <span v-if="loading[nodeId]" class="loading-indicator">...processing</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Wrapper to reserve space -->
    <div class="status-loading-placeholder">
      <div v-if="statusLoading" class="status-loading">Fetching node statuses...</div>
    </div>

    <div v-if="statusFetchError" class="alert error">
      <span class="alert-icon">!</span>
      {{ statusFetchError }}
    </div>
  </div>
</template>

<script>
import axios from 'axios';
import { API_BASE_URL } from '@/config';

const CONTROL_API_URL_BASE = `${API_BASE_URL}/control/node`;
const STATUS_API_URL = `${API_BASE_URL}/replication-summary`;
const LEADER_API_URL = `${API_BASE_URL}/current-leader`;
const STATUS_POLLING_INTERVAL = 5000;
const LEADER_POLLING_INTERVAL = 3000;

export default {
  name: 'NodeControl',
  data() {
    return {
      nodeIds: [1, 2, 3, 4],
      loading: {},
      message: '',
      messageType: 'info',
      nodeRunStatus: {},
      statusLoading: false,
      statusFetchError: null,
      statusIntervalId: null,
      currentLeaderId: -1,
      leaderIntervalId: null,
    };
  },
  methods: {
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
    async controlNode(nodeId, action) {
      this.loading[nodeId] = true;
      this.message = `Sending '${action}' request for Node ${nodeId}...`;
      this.messageType = 'info';

      try {
        const response = await axios.post(`${CONTROL_API_URL_BASE}/${nodeId}/${action}`);
        this.message = response.data.message || `Node ${nodeId} ${action} request successful.`;
        this.messageType = 'success';
        console.log(`Node ${nodeId} ${action} response:`, response.data);

        setTimeout(() => {
          this.fetchRunStatus();
          this.fetchCurrentLeader();
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
    async fetchCurrentLeader() {
        try {
            const response = await axios.get(LEADER_API_URL);
            this.currentLeaderId = response.data.currentLeaderId ?? -1;
        } catch (err) {
             console.error("Failed to fetch current leader:", err);
        }
    },
    startLeaderPolling() {
        this.stopLeaderPolling();
        this.fetchCurrentLeader();
        this.leaderIntervalId = setInterval(this.fetchCurrentLeader, LEADER_POLLING_INTERVAL);
    },
    stopLeaderPolling() {
         if (this.leaderIntervalId) {
             clearInterval(this.leaderIntervalId);
             this.leaderIntervalId = null;
         }
    }
  },
  created() {
    this.nodeIds.forEach(id => {
       this.loading[id] = false;
    });
  },
  mounted() {
      this.startStatusPolling();
      this.startLeaderPolling();
  },
  beforeUnmount() {
      this.stopStatusPolling();
      this.stopLeaderPolling();
  }
};
</script>

<style scoped>
.form-container {
  max-width: 800px;
  padding: 20px;
  background: white;
  border-radius: 5px;
  box-shadow: 0 2px 5px rgba(0,0,0,0.1);
  margin: 0; /* Left-aligned rather than centered */
}

h2 {
  margin-top: 0;
  color: #333;
  margin-bottom: 20px;
}

.leader-display {
  margin-bottom: 15px;
  padding: 10px;
  background-color: #e3f2fd;
  border-left: 4px solid #2196F3;
  border-radius: 4px;
}

.leader-display strong {
  color: #1565C0;
}

.warning-message {
  color: #8a6d3b;
  background-color: #fcf8e3;
  border: 1px solid #faebcc;
  padding: 10px;
  border-radius: 4px;
  margin-bottom: 15px;
  font-size: 0.9em;
}

.alert {
  margin-bottom: 15px;
  padding: 10px;
  border-radius: 4px;
  display: flex;
  align-items: center;
}

.alert-icon {
  margin-right: 8px;
  font-weight: bold;
}

.info {
  background-color: #e7f3fe;
  color: #31708f;
  border-left: 4px solid #2196F3;
}

.success {
  background-color: #dff0d8;
  color: #3c763d;
  border-left: 4px solid #4CAF50;
}

.error {
  background-color: #ffebee;
  color: #c62828;
  border-left: 4px solid #f44336;
}

.table-container {
  border: 1px solid #ddd;
  border-radius: 4px;
  margin-top: 15px;
  overflow: hidden;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th, td {
  border: 1px solid #ddd;
  padding: 10px;
  text-align: left;
  vertical-align: middle;
}

th {
  background-color: #f2f2f2;
  font-weight: 500;
}

tr.leader-row td {
  background-color: #e3f2fd;
  font-weight: bold;
}

.actions-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.action-button {
  padding: 6px 12px;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 500;
  min-width: 70px;
}

.start-button {
  background-color: #4CAF50;
  color: white;
}

.start-button:hover:not(:disabled) {
  background-color: #3d8b40;
}

.stop-button {
  background-color: #f44336;
  color: white;
}

.stop-button:hover:not(:disabled) {
  background-color: #d32f2f;
}

.action-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.status-indicator {
  display: inline-block;
  padding: 4px 10px;
  border-radius: 12px;
  font-size: 0.85em;
  font-weight: bold;
  color: white;
  min-width: 70px;
  text-align: center;
}

.status-indicator.running {
  background-color: #4CAF50;
}

.status-indicator.stopped {
  background-color: #f44336;
}

.status-indicator.unknown {
  background-color: #757575;
}

.loading-indicator {
  font-style: italic;
  color: #666;
  margin-left: 10px;
}

/* New class for the placeholder wrapper */
.status-loading-placeholder {
  min-height: 1.2em; /* Reserves space roughly equivalent to one line of the text below */
  margin-top: 10px; /* Keeps the original top margin */
  /* You might need to adjust min-height based on font size and line-height */
}

/* Original class, now inside the placeholder */
.status-loading {
  font-size: 0.9em;
  font-style: italic;
  color: #666;
  /* margin-top: 10px; <-- Removed, now handled by placeholder */
}
</style>
