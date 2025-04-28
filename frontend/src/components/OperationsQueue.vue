<template>
  <div class="operations-container">
    <h2>Operations History</h2>
    
    <div class="controls">
      <button @click="refreshOperations" :disabled="loading" class="submit-button">
        Refresh
      </button>
      
      <div class="filter-controls">
        <label>
          <span>Filter:</span>
          <select v-model="filter" @change="applyFilter">
            <option value="">All Operations</option>
            <option value="start">Node Start</option>
            <option value="stop">Node Stop</option>
            <option value="reset">System Reset</option>
            <option value="insert">Database Insert</option>
            <option value="update">Database Update</option>
            <option value="delete">Database Delete</option>
          </select>
        </label>
      </div>
      
      <div v-if="loading" class="loading-indicator">Loading...</div>
    </div>
    
    <div class="operations-list" v-if="operations.length > 0">
      <div 
        v-for="(op, index) in operations" 
        :key="index" 
        class="operation-item"
        :class="getTypeClass(op.type)"
      >
        <div class="operation-time">{{ formatTime(op.timestamp) }}</div>
        <div class="operation-type">
          <strong>{{ getOperationTypeLabel(op) }}</strong>
          <div v-if="op.email" class="operation-details">
            User: {{ op.email }}
          </div>
          <div v-if="op.table" class="operation-details">
            Table: {{ op.table }}
          </div>
        </div>
        <div class="operation-status">
          <span class="status-badge" :class="getStatusClass(op.status)">
            {{ op.status }}
          </span>
          <span v-if="op.targetNode > 0" class="target-node">
            → Node {{ op.targetNode }}
          </span>
          <span v-if="op.message" class="operation-message">
            {{ op.message }}
          </span>
        </div>
      </div>
    </div>
    
    <p v-else-if="!loading" class="empty-message">No operations matching the current filter.</p>
    
    <div v-if="error" class="alert error">
      <span class="alert-icon">!</span>
      {{ error }}
    </div>
  </div>
</template>

<script>
import axios from 'axios';
import { API_BASE_URL } from '@/config';

const OPERATIONS_URL = `${API_BASE_URL}/operations`;

export default {
  name: 'OperationsQueue',
  data() {
    return {
      operations: [],
      loading: false,
      error: null,
      pollingInterval: null,
      filter: '', // No filter by default
    };
  },
  methods: {
    async refreshOperations() {
      this.loading = true;
      this.error = null;
      
      try {
        let url = OPERATIONS_URL;
        if (this.filter) {
          url += `?type=${this.filter}`;
        }
        
        const response = await axios.get(url);
        this.operations = response.data.operations || [];
      } catch (err) {
        this.error = `Failed to fetch operations: ${err.message}`;
        console.error('Error fetching operations:', err);
      } finally {
        this.loading = false;
      }
    },
    getOperationTypeLabel(op) {
      switch(op.type) {
        case 'reset': 
          return 'System Reset';
        case 'start': 
          return `Start Node ${op.nodeId}`;
        case 'stop': 
          return `Stop Node ${op.nodeId}`;
        case 'insert': 
          return `Insert into ${op.table}`;
        case 'update': 
          return `Update ${op.table}`;
        case 'delete': 
          return `Delete from ${op.table}`;
        default:
          return op.type;
      }
    },
    getStatusClass(status) {
      switch (status) {
        case 'completed': return 'success';
        case 'failed': return 'error';
        case 'pending': return 'pending';
        case 'processing': return 'processing';
        case 'partial_success': return 'warning';
        default: return '';
      }
    },
    getTypeClass(type) {
      switch (type) {
        case 'insert': 
        case 'update': 
        case 'delete': 
          return 'db-operation';
        case 'start': 
        case 'stop': 
          return 'node-operation';
        case 'reset': 
          return 'reset-operation';
        default: 
          return '';
      }
    },
    formatTime(timestamp) {
      if (!timestamp) return "N/A";
      const date = new Date(timestamp);
      return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    },
    applyFilter() {
      this.refreshOperations();
    },
    startPolling() {
      this.refreshOperations();
      this.pollingInterval = setInterval(this.refreshOperations, 5000);
    },
    stopPolling() {
      if (this.pollingInterval) {
        clearInterval(this.pollingInterval);
        this.pollingInterval = null;
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
.operations-container {
  max-width: 800px;
  padding: 20px;
  background: white;
  border-radius: 5px;
  box-shadow: 0 2px 5px rgba(0,0,0,0.1);
  margin-top: 20px;
}

h2 {
  margin-top: 0;
  color: #333;
  margin-bottom: 20px;
}

.controls {
  display: flex;
  align-items: center;
  margin-bottom: 15px;
  flex-wrap: wrap;
  gap: 10px;
}

.filter-controls {
  display: flex;
  align-items: center;
  margin-left: 15px;
}

.filter-controls span {
  margin-right: 8px;
  font-weight: 500;
}

.filter-controls select {
  padding: 5px;
  border: 1px solid #ddd;
  border-radius: 4px;
  background-color: white;
}

.loading-indicator {
  margin-left: auto;
  color: #666;
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

.submit-button:hover:not(:disabled) {
  background-color: #3a7bc8;
}

.submit-button:disabled {
  background-color: #a0c4e8;
  cursor: not-allowed;
}

.operations-list {
  border: 1px solid #ddd;
  border-radius: 4px;
  margin-top: 15px;
  max-height: 400px;
  overflow-y: auto;
}

.operation-item {
  display: flex;
  padding: 12px 15px;
  border-bottom: 1px solid #eee;
  align-items: flex-start;
}

.operation-item:last-child {
  border-bottom: none;
}

.operation-item.db-operation {
  background-color: #f1f8e9;
}

.operation-item.node-operation {
  background-color: #e8eaf6;
}

.operation-item.reset-operation {
  background-color: #fbe9e7;
}

.operation-time {
  width: 100px;
  color: #666;
  font-size: 0.9em;
  flex-shrink: 0;
}

.operation-type {
  flex: 1;
  padding-right: 15px;
}

.operation-details {
  font-size: 0.85em;
  color: #666;
  margin-top: 3px;
}

.operation-status {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.status-badge {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 12px;
  font-size: 0.8em;
  font-weight: bold;
  color: white;
  margin-right: 10px;
}

.success {
  background-color: #4caf50;
}

.error {
  background-color: #f44336;
}

.pending {
  background-color: #2196f3;
}

.processing {
  background-color: #ff9800;
}

.warning {
  background-color: #ff9800;
}

.target-node {
  font-size: 0.9em;
  background-color: #e0e0e0;
  padding: 2px 6px;
  border-radius: 4px;
  margin-right: 8px;
  white-space: nowrap;
}

.operation-message {
  font-size: 0.9em;
  color: #666;
  font-style: italic;
  max-width: 250px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-message {
  padding: 15px;
  background-color: #f8f8f8;
  border-radius: 4px;
  color: #666;
  font-style: italic;
  text-align: center;
}

.alert {
  margin-top: 15px;
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