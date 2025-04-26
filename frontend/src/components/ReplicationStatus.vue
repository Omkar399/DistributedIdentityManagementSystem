<template>
  <div class="form-container">
    <h2>Replication Status</h2>
    
    <div class="controls">
      <button ref="refreshButton" @click="fetchStatus" :disabled="loading" class="submit-button">
        Refresh Status
      </button>
    </div>

    <div v-if="fetchError" class="alert error">
      <span class="alert-icon">!</span>
      {{ fetchError }}
    </div>

    <!-- Skeleton Loader during loading -->
    <div v-if="loading" class="skeleton-loader">
      <div class="skeleton-item" v-for="n in 4" :key="`skel-${n}`"></div>
    </div>
    
    <div v-else class="status-container">
      <ul v-if="Object.keys(nodeStatus).length > 0" class="status-list">
        <li v-for="(status, nodeId) in sortedNodeStatus" :key="nodeId" class="status-item">
          <div class="node-label">Node {{ nodeId }}:</div>
          <div class="status-content">
            <span v-if="status.error" class="status-error">
              Error: {{ status.error }} 
              <span class="status-icon error-icon">✗</span>
            </span>
            <span v-else class="status-details">
              Last Log ID = {{ status.lastLogId }}
              <span class="timestamp">({{ formatTimestamp(status.lastLogTimestamp) }})</span>
              <span :class="['status-icon', getStatusClass(status.lastLogId)]">
                {{ getStatusIcon(status.lastLogId) }}
              </span>
            </span>
          </div>
        </li>
      </ul>
      <div v-else class="empty-message">No status data available.</div>
    </div>
  </div>
</template>

<script>
import axios from 'axios';

const STATUS_API_URL = 'http://localhost:8090/replication-summary';
const POLLING_INTERVAL = 10000;

export default {
  name: 'ReplicationStatus',
  data() {
    return {
      nodeStatus: {},
      maxLogId: 0,
      loading: false,
      fetchError: null,
      intervalId: null,
    };
  },
  computed: {
    sortedNodeStatus() {
      const sortedKeys = Object.keys(this.nodeStatus).sort((a, b) => parseInt(a) - parseInt(b));
      const sorted = {};
      for (const key of sortedKeys) {
        sorted[key] = this.nodeStatus[key];
      }
      return sorted;
    }
  },
  methods: {
    async fetchStatus() {
      this.loading = true;
      this.fetchError = null;
      try {
        const response = await axios.get(STATUS_API_URL);
        this.nodeStatus = response.data.nodes || {};
        this.maxLogId = 0;
        for (const nodeId in this.nodeStatus) {
          const status = this.nodeStatus[nodeId];
          if (!status.error && status.lastLogId > this.maxLogId) {
            this.maxLogId = status.lastLogId;
          }
        }
      } catch (err) {
        this.fetchError = `Failed to fetch replication status: ${err.response?.data?.message || err.message}`;
        console.error("Fetch Status Error:", err);
        this.nodeStatus = {};
        this.maxLogId = 0;
      } finally {
        this.loading = false;
        this.$nextTick(() => {
          this.$refs.refreshButton?.blur();
        });
      }
    },
    getStatusIcon(lastLogId) {
      if (lastLogId === null || lastLogId === undefined) return '❓';
      if (lastLogId >= this.maxLogId) return '✓';
      if (this.maxLogId > 0 && lastLogId >= this.maxLogId - 5) return '⏳';
      if (this.maxLogId > 0) return '✗';
      return '✓';
    },
    getStatusClass(lastLogId) {
      if (lastLogId === null || lastLogId === undefined) return 'unknown';
      if (lastLogId >= this.maxLogId) return 'success';
      if (this.maxLogId > 0 && lastLogId >= this.maxLogId - 5) return 'warning';
      if (this.maxLogId > 0) return 'error';
      return 'success';
    },
    formatTimestamp(timestampString) {
      if (!timestampString || timestampString === '0001-01-01T00:00:00Z') {
        return 'N/A';
      }
      try {
        const date = new Date(timestampString);
        return date.toLocaleString(undefined, {
          year: 'numeric', month: 'numeric', day: 'numeric',
          hour: '2-digit', minute: '2-digit', second: '2-digit',
          fractionalSecondDigits: 3,
          hour12: false
        });
      } catch (e) {
        console.error("Error parsing timestamp:", timestampString, e);
        return 'Invalid Date';
      }
    },
    startPolling() {
      this.stopPolling();
      this.fetchStatus();
      this.intervalId = setInterval(this.fetchStatus, POLLING_INTERVAL);
    },
    stopPolling() {
      if (this.intervalId) {
        clearInterval(this.intervalId);
        this.intervalId = null;
      }
    }
  },
  mounted() {
    this.startPolling();
  },
  beforeUnmount() {
    this.stopPolling();
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

.controls {
  margin-bottom: 15px;
}

.submit-button {
  padding: 8px 16px;
  background-color: #4a90e2;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 500;
}

.submit-button:hover {
  background-color: #3a7bc8;
}

.submit-button:disabled {
  background-color: #a0c4e8;
  cursor: not-allowed;
}

.status-container {
  border: 1px solid #ddd;
  border-radius: 4px;
  margin-top: 15px;
  overflow: hidden;
}

.status-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.status-item {
  display: flex;
  padding: 12px 15px;
  border-bottom: 1px solid #eee;
  background-color: #fff;
}

.status-item:last-child {
  border-bottom: none;
}

.node-label {
  font-weight: 500;
  width: 100px;
}

.status-content {
  flex: 1;
}

.status-details {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
}

.timestamp {
  font-size: 0.85em;
  color: #666;
  margin-left: 5px;
}

.status-icon {
  font-weight: bold;
  margin-left: 8px;
  font-size: 1.1em;
}

.status-icon.success {
  color: #4CAF50;
}

.status-icon.warning {
  color: #FF9800;
}

.status-icon.error {
  color: #f44336;
}

.status-icon.unknown {
  color: #757575;
}

.status-error {
  color: #f44336;
  font-style: italic;
}

.error-icon {
  color: #f44336;
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

.error {
  background-color: #ffebee;
  color: #c62828;
  border-left: 4px solid #f44336;
}

.empty-message {
  padding: 15px;
  text-align: left;
  font-style: italic;
  color: #666;
}

/* Skeleton loader styles */
.skeleton-loader {
  padding: 10px;
}

.skeleton-item {
  height: 40px;
  margin-bottom: 10px;
  background: linear-gradient(90deg, #f0f0f0 25%, #e0e0e0 50%, #f0f0f0 75%);
  background-size: 200% 100%;
  animation: loading 1.5s infinite;
  border-radius: 4px;
}

@keyframes loading {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
</style>