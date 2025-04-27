// Detect if we're in development or production
const isDevelopment = process.env.NODE_ENV === 'development';

// Backend URLs
export const API_BASE_URL = isDevelopment 
  ? 'http://13.218.75.79:8090'  // Direct connection in development 
  : '/api/proxy';               // Proxy in production

export const API_REGION = 'asia';

export default {
  API_BASE_URL,
  API_REGION,
};