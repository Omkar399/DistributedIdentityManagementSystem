<template>
  <div class="form-container">
    <h2>Transaction Logs</h2>
    <div class="controls">
      <button @click="fetchLogs" class="submit-button">Refresh Logs</button>
    </div>
    
    <div v-if="error" class="alert error">
      <span class="alert-icon">!</span>
      {{ error }}
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th class="sticky-header">ID</th>
            <th class="sticky-header">Timestamp</th>
            <th class="sticky-header">Type</th>
            <th class="sticky-header">Table</th>
            <th class="sticky-header">Query</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="logs.length === 0 && !error">
            <td colspan="5" class="empty-message">No logs found.</td>
          </tr>
          <tr v-for="log in logs" :key="log.id">
            <td>{{ log.id }}</td>
            <td>{{ formatTimestamp(log.timestamp) }}</td>
            <td>{{ log.type }}</td>
            <td>{{ log.table_name }}</td>
            <td><pre>{{ log.query }}</pre></td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script>
import api from '@/services/api';

export default {
  name: 'LogViewer',
  data() {
    return {
      logs: [],
      error: null,
    };
  },
  methods: {
    async fetchLogs() {
      this.error = null;
      try {
        const response = await api.getLogs();
        this.logs = response.data?.sort((a, b) => b.id - a.id) || [];
      } catch (err) {
        this.error = `Failed to fetch logs: ${err.response?.data?.message || err.message}`;
        console.error(err);
        this.logs = [];
      }
    },
    formatTimestamp(timestamp) {
      if (!timestamp) return 'N/A';
      try {
        const date = new Date(timestamp);
        return date.toLocaleString(undefined, {
          year: 'numeric', 
          month: 'numeric', 
          day: 'numeric',
          hour: '2-digit', 
          minute: '2-digit', 
          second: '2-digit',
          fractionalSecondDigits: 3,
          hour12: false
        });
      } catch (e) {
        console.error("Error formatting timestamp:", timestamp, e);
        return 'Invalid Date';
      }
    }
  },
  mounted() {
    this.fetchLogs();
  }
};
</script>

<style scoped>
.form-container {
  max-width: 900px;
  padding: 20px;
  background: white;
  border-radius: 5px;
  box-shadow: 0 2px 5px rgba(0,0,0,0.1);
  margin: 0; /* Changed from margin: 0 auto to align left */
}

h2 {
  margin-top: 0;
  color: #333;
  margin-bottom: 20px;
  text-align: left; /* Explicitly set text alignment */
}

.controls {
  margin-bottom: 15px;
  text-align: left; /* Explicitly set text alignment */
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

.table-container {
  max-height: 400px;
  overflow-y: auto;
  border: 1px solid #ddd;
  border-radius: 4px;
  margin-top: 10px;
  position: relative;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th, td {
  border: 1px solid #ddd;
  padding: 10px;
  text-align: left;
  vertical-align: top;
}

.sticky-header {
  position: sticky;
  top: 0;
  background-color: #f2f2f2;
  z-index: 10;
  font-weight: 500;
}

pre {
  white-space: pre-wrap;
  word-wrap: break-word;
  margin: 0;
  font-family: monospace;
  font-size: 0.9em;
}

.empty-message {
  text-align: left; /* Changed from center to left alignment */
  font-style: italic;
  color: #666;
  padding: 15px;
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
</style>