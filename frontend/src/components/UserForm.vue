<template>
  <div>
    <h2>Create New User</h2>
    <form @submit.prevent="createUser">
      <div>
        <label for="email">Email:</label>
        <input type="email" id="email" v-model="newUser.email" required>
      </div>
      <div>
        <label for="password">Password:</label>
        <input type="password" id="password" v-model="newUser.password" required>
        <small>Warning: Password will be sent as provided. Ensure backend handles hashing if needed.</small>
      </div>
      <div>Permissions:</div>
       <label><input type="checkbox" v-model="newUser.R1"> R1</label>
       <label><input type="checkbox" v-model="newUser.R2"> R2</label>
       <label><input type="checkbox" v-model="newUser.R3"> R3</label>
       <label><input type="checkbox" v-model="newUser.R4"> R4</label>

      <button type="submit">Create User</button>
    </form>
     <p v-if="message">{{ message }}</p>
     <p v-if="error">{{ error }}</p>
  </div>
</template>

<script>
import api from '@/services/api';

export default {
  name: 'UserForm',
  data() {
    return {
      newUser: {
        email: '',
        password: '',
        R1: false,
        R2: false,
        R3: false,
        R4: false,
      },
      message: '',
      error: '',
    };
  },
  methods: {
    async createUser() {
      this.message = '';
      this.error = '';
      try {
        // Ensure boolean permissions are included correctly
        const userData = {
            email: this.newUser.email,
            password: this.newUser.password,
            R1: this.newUser.R1,
            R2: this.newUser.R2,
            R3: this.newUser.R3,
            R4: this.newUser.R4,
        };
        const response = await api.createUser(userData);
        this.message = `User created successfully. Rows affected: ${response.data.rows_affected}`;
        // Optionally clear form or emit event
        this.newUser = { email: '', password: '', R1: false, R2: false, R3: false, R4: false };
        this.$emit('user-created'); // Notify parent component if needed
      } catch (err) {
         this.error = `Failed to create user: ${err.response?.data?.message || err.message}`;
         console.error(err);
      }
    },
  },
};
</script>

<style scoped>
form div { margin-bottom: 10px; }
label { margin-right: 5px; }
small { display: block; color: grey; }
</style>
