import axios from 'axios';
import { API_BASE_URL, API_REGION } from '@/config';

// Use the middleware address and port
// The middleware requires a region in the path, e.g., /asia/ or /usa/
// Ensure this matches your middleware setup and CORS configuration
const API_URL = `${API_BASE_URL}/${API_REGION}`; 

const apiClient = axios.create({
  baseURL: API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Function to interact with the /query endpoint
const executeQuery = (queryData) => {
  return apiClient.post('/query', queryData);
};

export default {
  // Fetch all users
  getUsers() {
    const query = {
      type: 'SELECT',
      table: 'users',
      // Specify fields, assuming backend returns lowercase keys for JSON standard
      fields: ['email', 'r1', 'r2', 'r3', 'r4'],
    };
    return executeQuery(query);
  },

  // Create a new user
  // Backend expects password string and stringified booleans for roles
  createUser(userData) {
    // userData should contain { email, password, R1, R2, R3, R4 } with boolean values
    const values = {
      email: userData.email,
      password: userData.password, // Send password string
      // Send uppercase keys with stringified booleans as expected by backend INSERT/UPDATE
      R1: String(userData.R1),
      R2: String(userData.R2),
      R3: String(userData.R3),
      R4: String(userData.R4),
    };
    const query = {
      type: 'INSERT',
      table: 'users',
      values: values,
    };
    console.log('Sending create query:', query);
    return executeQuery(query);
  },

  // Update user permissions
  updateUserPermissions(email, permissions) {
    // permissions will be { R1: true } or { R2: false }, etc. (uppercase keys)
    const values = {};
    // Iterate over the keys provided in the permissions object
    for (const key in permissions) {
      if (Object.hasOwnProperty.call(permissions, key)) {
        // Add the key (e.g., "R1") and its stringified boolean value (e.g., "true")
        // This matches the backend expectation for R1, R2, R3, R4 fields in buildUpdateQuery[3]
        values[key] = String(permissions[key]);
      }
    }

    const query = {
      type: 'UPDATE',
      table: 'users',
      values: values, // values should now be correctly populated e.g., { "R1": "true" }
      where: {
        // Assuming 'email' is the correct field for the WHERE clause based on backend[3]
        email: email,
      },
    };
    console.log('Sending update query:', query); // Log the exact payload
    return executeQuery(query);
  },

  // Fetch transaction logs
  getLogs() {
    const query = {
      type: 'SELECT',
      table: 'transaction_log',
      // Adjust fields if needed based on actual table structure
      fields: ['id', 'type', 'table_name', 'query', 'timestamp'],
      // Consider adding ORDER BY timestamp DESC or id DESC if desired
    };
    return executeQuery(query);
  },
};
