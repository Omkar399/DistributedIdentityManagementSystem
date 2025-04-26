<template>
  <div>
    <h2>User List</h2>
    <button @click="fetchUsers" :disabled="loading">Refresh Users</button>
    <div v-if="loading">Loading...</div>
    <table v-else-if="users.length > 0">
      <thead>
        <tr>
          <th>Email</th>
          <th>R1</th> <!-- Table header -->
          <th>R2</th> <!-- Table header -->
          <th>R3</th> <!-- Table header -->
          <th>R4</th> <!-- Table header -->
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(user, index) in users" :key="user.email">
          <td>{{ user.email }}</td>
          <!-- Bind checkbox state to lowercase keys (from SELECT data) -->
          <!-- Pass uppercase keys to togglePermission for the UPDATE payload -->
          <td><input type="checkbox" :checked="user.r1" @change="togglePermission(index, 'R1', $event)"></td>
          <td><input type="checkbox" :checked="user.r2" @change="togglePermission(index, 'R2', $event)"></td>
          <td><input type="checkbox" :checked="user.r3" @change="togglePermission(index, 'R3', $event)"></td>
          <td><input type="checkbox" :checked="user.r4" @change="togglePermission(index, 'R4', $event)"></td>
          <td><!-- Optional: Add Edit/Delete buttons --></td>
        </tr>
      </tbody>
    </table>
    <p v-else>No users found.</p>
    <p v-if="error" style="color: red;">{{ error }}</p>
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
table { width: 100%; border-collapse: collapse; margin-top: 10px;}
th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
thead { background-color: #f2f2f2; }
p[style*="color: red"] { margin-top: 10px; font-weight: bold; }
</style>
