<template>
  <div class="log-viewer-container"> <!-- Optional overall container -->
    <h2>Transaction Logs</h2>
    <button @click="fetchLogs">Refresh Logs</button>
    <p v-if="error" class="error-message">{{ error }}</p>

    <!-- Add a scrollable container around the table -->
    <div class="log-table-container">
      <table>
        <thead>
          <tr>
            <!-- Add sticky class to headers -->
            <th class="sticky-header">ID</th>
            <th class="sticky-header">Timestamp</th>
            <th class="sticky-header">Type</th>
            <th class="sticky-header">Table</th>
            <th class="sticky-header">Query</th>
          </tr>
        </thead>
        <tbody>
          <!-- Show message if no logs -->
          <tr v-if="logs.length === 0 && !error">
            <td colspan="5" style="text-align: center; font-style: italic;">No logs found.</td>
          </tr>
          <!-- Render log rows -->
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
     <!-- Error moved above table container -->
  </div>
</template>

<script>
import api from '@/services/api'; // Assuming this service exists and works

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
        // Assuming api.getLogs() fetches from your middleware/leader
        const response = await api.getLogs();
        // Sort logs by ID descending to show newest first
        this.logs = response.data?.sort((a, b) => b.id - a.id) || [];
      } catch (err) {
        this.error = `Failed to fetch logs: ${err.response?.data?.message || err.message}`;
        console.error(err);
        this.logs = []; // Clear logs on error
      }
    },
     formatTimestamp(timestamp) {
        if (!timestamp) return 'N/A';
        try {
            // Use a more robust format including milliseconds if available
            const date = new Date(timestamp);
            return date.toLocaleString(undefined, {
                year: 'numeric', month: 'numeric', day: 'numeric',
                hour: '2-digit', minute: '2-digit', second: '2-digit',
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
    // Optional: Add polling if you want automatic refresh
    // this.intervalId = setInterval(this.fetchLogs, 15000); // e.g., every 15 seconds
  },
  // Optional: Clear interval if polling
  // beforeUnmount() {
  //   if (this.intervalId) {
  //     clearInterval(this.intervalId);
  //   }
  // }
};
</script>

<style scoped>
.log-viewer-container {
    margin-top: 20px; /* Add some spacing */
}

/* Style the scrollable container */
.log-table-container {
  max-height: 400px; /* Set a fixed maximum height */
  overflow-y: auto; /* Enable vertical scrolling only when needed */
  border: 1px solid #ccc; /* Add a border for visual clarity */
  margin-top: 10px; /* Space below the button */
  position: relative; /* Needed for sticky header positioning context */
}

table {
  width: 100%;
  border-collapse: collapse;
}
th, td {
  border: 1px solid #ddd;
  padding: 8px;
  text-align: left;
  vertical-align: top; /* Align content to the top of cells */
}

/* Sticky Table Header */
thead th.sticky-header {
  position: sticky;
  top: 0; /* Stick to the top of the scrollable container */
  background-color: #f2f2f2; /* Match original header background */
  z-index: 10; /* Ensure header stays above table rows */
}

pre {
  white-space: pre-wrap; /* Wrap long query lines */
  word-wrap: break-word; /* Break long words if necessary */
  margin: 0; /* Remove default pre margins */
  font-family: monospace; /* Use a monospace font for queries */
  font-size: 0.9em; /* Slightly smaller font for queries */
}

.error-message {
    color: #a94442; /* Match error message color */
    background-color: #f2dede;
    border: 1px solid #ebccd1;
    padding: 10px;
    border-radius: 4px;
    margin-top: 10px;
}
</style>
