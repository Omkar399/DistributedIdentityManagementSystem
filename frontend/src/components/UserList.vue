<template>
  <div class="form-container">
    <h2>User List</h2>
    <div class="controls">
      <button @click="fetchUsers" :disabled="loading" class="submit-button">Refresh Users</button>
      <div v-if="loading" class="loading-indicator">Loading...</div>
    </div>
    
    <div class="table-container" v-if="users.length > 0">
      <table>
        <thead>
          <tr>
            <th>Email</th>
            <th>R1</th>
            <th>R2</th>
            <th>R3</th>
            <th>R4</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(user, index) in users" :key="user.email">
            <td>{{ user.email }}</td>
            <td class="checkbox-cell"><input type="checkbox" :checked="user.r1" @change="togglePermission(index, 'R1', $event)"></td>
            <td class="checkbox-cell"><input type="checkbox" :checked="user.r2" @change="togglePermission(index, 'R2', $event)"></td>
            <td class="checkbox-cell"><input type="checkbox" :checked="user.r3" @change="togglePermission(index, 'R3', $event)"></td>
            <td class="checkbox-cell"><input type="checkbox" :checked="user.r4" @change="togglePermission(index, 'R4', $event)"></td>
            <td><!-- Optional: Add Edit/Delete buttons --></td>
          </tr>
        </tbody>
      </table>
    </div>
    
    <p v-else-if="!loading" class="empty-message">No users found.</p>
    
    <div v-if="error" class="alert error">
      <span class="alert-icon">!</span>
      {{ error }}
    </div>
  </div>
</template>

<script>
import api from '@/services/api';

export default {
  name: 'UserList',
  data() {
    return {
      users: [],
      error: null,
      loading: false,
    };
  },
  methods: {
    async fetchUsers() {
      this.error = null;
      this.loading = true;
      this.users = []; // Clear previous users
      try {
        const response = await api.getUsers();
        // Assuming response.data is the array of user objects with lowercase boolean keys
        this.users = response.data;
        console.log('Fetched users:', JSON.parse(JSON.stringify(this.users))); // Deep copy for logging
      } catch (err) {
        this.error = `Failed to fetch users: ${err.response?.data?.message || err.message}`;
        console.error(err);
      } finally {
          this.loading = false;
      }
    },
    // Pass the index and uppercase permission key
    async togglePermission(userIndex, permissionKey, event) {
      this.error = null;
      const user = this.users[userIndex];
      if (!user) return; // Safety check

      // Get the new state directly from the event target
      const newPermissionState = event.target.checked;

      // Prepare the payload for the API call using the UPPERCASE key
      const permissionsToUpdate = { [permissionKey]: newPermissionState };

      // --- Local State Update ---
      // Convert uppercase key to lowercase for local state consistency
      const localKey = permissionKey.toLowerCase();
      // Optimistically update local state for responsiveness
      const originalLocalState = user[localKey]; // Store original state for rollback
      this.users[userIndex][localKey] = newPermissionState;
      // --- End Local State Update ---


      try {
        // Pass uppercase permission key and boolean value to the API service
        await api.updateUserPermissions(user.email, permissionsToUpdate);
        // API call successful, local state is already updated
        console.log(`Permission ${permissionKey} updated for ${user.email} to ${newPermissionState}`);
      } catch (err) {
        this.error = `Failed to update permission ${permissionKey} for ${user.email}: ${err.response?.data?.message || err.message}`;
        console.error(err);

        // --- Rollback Local State on Failure ---
        this.users[userIndex][localKey] = originalLocalState; // Revert local data
        event.target.checked = originalLocalState; // Revert the checkbox visual state
        // --- End Rollback ---
      }
    },
  },
  mounted() {
    this.fetchUsers();
  },
};
</script>

<style scoped>
.form-container {
  max-width: 800px;
  padding: 20px;
  background: white;
  border-radius: 5px;
  box-shadow: 0 2px 5px rgba(0,0,0,0.1);
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
}

.loading-indicator {
  margin-left: 15px;
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

.submit-button:hover {
  background-color: #3a7bc8;
}

.submit-button:disabled {
  background-color: #a0c4e8;
  cursor: not-allowed;
}

.table-container {
  margin-top: 15px;
  border: 1px solid #ddd;
  border-radius: 4px;
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
}

th {
  background-color: #f2f2f2;
  font-weight: 500;
}

.checkbox-cell {
  text-align: center;
}

.checkbox-cell input[type="checkbox"] {
  width: 18px;
  height: 18px;
  cursor: pointer;
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