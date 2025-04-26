<template>
  <div>
    <h2>Transaction Logs</h2>
    <button @click="fetchLogs">Refresh Logs</button>
    <table>
      <thead>
        <tr>
          <th>ID</th>
          <th>Timestamp</th>
          <th>Type</th>
          <th>Table</th>
          <th>Query</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="log in logs" :key="log.id">
          <td>{{ log.id }}</td>
          <td>{{ formatTimestamp(log.timestamp) }}</td>
          <td>{{ log.type }}</td>
          <td>{{ log.table_name }}</td>
          <td><pre>{{ log.query }}</pre></td>
        </tr>
      </tbody>
    </table>
     <p v-if="error">{{ error }}</p>
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
        // Sort logs by ID descending to show newest first
        this.logs = response.data.sort((a, b) => b.id - a.id);
      } catch (err) {
        this.error = `Failed to fetch logs: ${err.response?.data?.message || err.message}`;
        console.error(err);
      }
    },
     formatTimestamp(timestamp) {
        if (!timestamp) return '';
        // Timestamps might be in a specific format from Postgres, adjust parsing if needed
        return new Date(timestamp).toLocaleString();
     }
  },
  mounted() {
    this.fetchLogs();
  },
};
</script>

<style scoped>
table { width: 100%; border-collapse: collapse; }
th, td { border: 1px solid #ddd; padding: 8px; text-align: left; vertical-align: top;}
thead { background-color: #f2f2f2; }
pre { white-space: pre-wrap; word-wrap: break-word; margin: 0; }
</style>
