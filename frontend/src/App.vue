<template>
  <div id="app">
    <h1>Distributed Identity Management</h1>
    <UserForm @user-created="refreshData" />
    <hr>
    <UserList ref="userList" />
    <hr>
    <LogViewer ref="logViewer" />
    <hr>
    <ReplicationStatus ref="replicationStatus" /> <!-- Add the component -->
  </div>
</template>

<script>
import UserList from './components/UserList.vue';
import UserForm from './components/UserForm.vue';
import LogViewer from './components/LogViewer.vue';
import ReplicationStatus from './components/ReplicationStatus.vue'; // Import

export default {
  name: 'App',
  components: {
    UserList,
    UserForm,
    LogViewer,
    ReplicationStatus, // Register
  },
  methods: {
      // Renamed for clarity
      refreshData() {
          // Call the fetchUsers method on the UserList component
          this.$refs.userList.fetchUsers();
          // Refresh logs as user creation adds a log entry
          this.$refs.logViewer.fetchLogs();
          // Also refresh replication status as logs have changed
          this.$refs.replicationStatus.fetchStatus();
      }
  }
};
</script>

<style>
#app { font-family: Avenir, Helvetica, Arial, sans-serif; padding: 20px; max-width: 900px; margin: auto;}
hr { margin: 25px 0; border: 0; border-top: 1px solid #eee;}
</style>
