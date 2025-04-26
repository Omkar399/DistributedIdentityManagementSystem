<template>
  <div id="app">
    <h1>Distributed Identity Management</h1>
    <UserForm @user-created="refreshData" />
    <hr>
    <NodeControl /> <!-- Add the new component -->
    <hr>
    <UserList ref="userList" />
    <hr>
    <LogViewer ref="logViewer" />
    <hr>
    <ReplicationStatus ref="replicationStatus" />
  </div>
</template>

<script>
import UserList from './components/UserList.vue';
import UserForm from './components/UserForm.vue';
import LogViewer from './components/LogViewer.vue';
import ReplicationStatus from './components/ReplicationStatus.vue';
import NodeControl from './components/NodeControl.vue'; // Import

export default {
  name: 'App',
  components: {
    UserList,
    UserForm,
    LogViewer,
    ReplicationStatus,
    NodeControl, // Register
  },
  methods: {
      refreshData() {
          this.$refs.userList.fetchUsers();
          this.$refs.logViewer.fetchLogs();
          // No need to manually refresh replication status here, polling handles it
          // this.$refs.replicationStatus.fetchStatus();
      }
  }
};
</script>

<style>
/* ... existing styles ... */
</style>
