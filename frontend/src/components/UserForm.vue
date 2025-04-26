<template>
  <div class="form-container">
    <h2>Create New User</h2>
    <form @submit.prevent="createUser">
      <div class="form-group">
        <label for="email">Email:</label>
        <input type="email" id="email" v-model="newUser.email" required class="form-input">
      </div>
      <div class="form-group">
        <label for="password">Password:</label>
        <input type="password" id="password" v-model="newUser.password" required class="form-input">
        <small>Warning: Password will be sent as provided. Ensure backend handles hashing if needed.</small>
      </div>
      <div class="form-group">
        <div class="permissions-label">Permissions:</div>
        <div class="permissions-row">
          <label class="permission-item"><input type="checkbox" v-model="newUser.R1"> R1</label>
          <label class="permission-item"><input type="checkbox" v-model="newUser.R2"> R2</label>
          <label class="permission-item"><input type="checkbox" v-model="newUser.R3"> R3</label>
          <label class="permission-item"><input type="checkbox" v-model="newUser.R4"> R4</label>
        </div>
      </div>

      <button type="submit" class="submit-button">Create User</button>
    </form>
    <div class="message-container">
      <p v-if="message" class="success-message">{{ message }}</p>
      <p v-if="error" class="error-message">{{ error }}</p>
    </div>
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
.form-container {
  max-width: 500px;
  padding: 20px;
  background: white;
  border-radius: 5px;
  box-shadow: 0 2px 5px rgba(0,0,0,0.1);
}

h2 {
  margin-top: 0;
  color: #333;
}

.form-group {
  margin-bottom: 15px;
}

.form-input {
  width: 100%;
  padding: 8px;
  border: 1px solid #ddd;
  border-radius: 4px;
  box-sizing: border-box;
}

.permissions-label {
  margin-bottom: 8px;
  font-weight: 500;
}

.permissions-row {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
}

.permission-item {
  display: flex;
  align-items: center;
  cursor: pointer;
}

.permission-item input[type="checkbox"] {
  margin-right: 5px;
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

.message-container {
  margin-top: 15px;
}

.success-message {
  padding: 8px;
  background-color: #e6f7e6;
  color: #2c662d;
  border-radius: 4px;
}

.error-message {
  padding: 8px;
  background-color: #ffebee;
  color: #c62828;
  border-radius: 4px;
}

small {
  display: block;
  color: grey;
  margin-top: 5px;
}
</style>