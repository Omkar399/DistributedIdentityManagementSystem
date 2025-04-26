<template>
  <div class="replication-status-container">
    <h3>Replication Status</h3>
    <!-- Add ref="refreshButton" -->
    <button ref="refreshButton" @click="fetchStatus" :disabled="loading">Refresh Status</button>

    <!-- Skeleton Loader or Content Area -->
    <!-- ... rest of template from previous skeleton loader example ... -->
    <div v-if="loading" class="skeleton-loader">
      <div class="skeleton-item" v-for="n in 4" :key="`skel-${n}`"></div>
    </div>
    <div v-else>
      <ul v-if="Object.keys(nodeStatus).length > 0" class="status-list">
         <li v-for="(status, nodeId) in sortedNodeStatus" :key="nodeId" class="status-item">
          Node {{ nodeId }}:
          <span v-if="status.error" class="status-error">
             Error: {{ status.error }} <span class="status-icon" style="color: red;">✗</span>
          </span>
          <span v-else>
            Last Log ID = {{ status.lastLogId }}
            <span class="timestamp"> ({{ formatTimestamp(status.lastLogTimestamp) }})</span>
            <span :style="{ color: getStatusColor(status.lastLogId) }" class="status-icon">
              {{ getStatusIcon(status.lastLogId) }}
            </span>
          </span>
        </li>
      </ul>
      <div v-else class="no-data">No status data available.</div>
    </div>
    <p v-if="fetchError" class="fetch-error">{{ fetchError }}</p>
    <!-- ... -->
  </div>
</template>

<script>
// ... imports, data, computed, other methods ...
import axios from 'axios';

const STATUS_API_URL = 'http://localhost:8090/replication-summary';
const POLLING_INTERVAL = 10000;

export default {
  name: 'ReplicationStatus',
  // ... data, computed ...
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
        // ... calculate maxLogId ...
        this.maxLogId = 0;
        for (const nodeId in this.nodeStatus) {
          const status = this.nodeStatus[nodeId];
          if (!status.error && status.lastLogId > this.maxLogId) {
            this.maxLogId = status.lastLogId;
          }
        }
      } catch (err) {
        this.fetchError = `Failed to fetch replication status: ${err.response?.data?.message || err.message}`;
        // ... error handling ...
        console.error("Fetch Status Error:", err);
        this.nodeStatus = {};
        this.maxLogId = 0;
      } finally {
        this.loading = false;
        // Use nextTick to ensure blur happens after potential DOM updates
        this.$nextTick(() => {
          this.$refs.refreshButton?.blur(); // Blur the button using its ref
        });
      }
    },
    // ... other methods (getStatusIcon, getStatusColor, formatTimestamp, etc.) ...
     getStatusIcon(lastLogId) {
       if (lastLogId === null || lastLogId === undefined) return '❓';
       if (lastLogId >= this.maxLogId) return '✓';
       if (this.maxLogId > 0 && lastLogId >= this.maxLogId - 5) return '⏳';
       if (this.maxLogId > 0) return '✗';
       return '✓';
    },
    getStatusColor(lastLogId) {
       if (lastLogId === null || lastLogId === undefined) return 'gray';
       if (lastLogId >= this.maxLogId) return 'green';
       if (this.maxLogId > 0 && lastLogId >= this.maxLogId - 5) return 'orange';
       if (this.maxLogId > 0) return 'red';
       return 'green';
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
  // ... mounted, beforeUnmount ...
   mounted() {
    this.startPolling();
  },
  beforeUnmount() {
    this.stopPolling();
  }
};
</script>

<style scoped>
/* ... styles from previous skeleton loader example ... */
.replication-status-container {
  border: 1px solid #ccc; padding: 15px; margin-top: 20px; border-radius: 5px; background-color: #f9f9f9;
  position: relative;
  overflow: hidden;
}
h3 { margin-top: 0; border-bottom: 1px solid #eee; padding-bottom: 10px; }
button { margin-bottom: 10px; }
.status-list, .skeleton-loader { list-style: none; padding: 0; margin: 0; }
.status-item, .skeleton-item, .no-data {
  margin-bottom: 8px; padding: 5px; border-radius: 3px; border: 1px solid #eee; min-height: 1.5em;
}
.status-item { background-color: #fff; }
.no-data { background-color: #fff; color: #555; font-style: italic; }
.skeleton-loader {}
.skeleton-item {
  background: linear-gradient(90deg, #f0f0f0 25%, #e0e0e0 50%, #f0f0f0 75%);
  background-size: 200% 100%;
  animation: loading 1.5s infinite;
  border-color: #e0e0e0; color: transparent; user-select: none;
}
@keyframes loading { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
.status-icon { font-weight: bold; margin-left: 8px; font-size: 1.1em; }
.timestamp { font-size: 0.85em; color: #555; margin-left: 5px; }
.status-error { color: red; font-style: italic; }
.fetch-error { color: red; font-weight: bold; margin-top: 10px; }
</style>
