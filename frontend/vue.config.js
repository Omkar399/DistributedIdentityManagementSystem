// frontend/vue.config.js
const path = require('path');

module.exports = {
  // Add the devServer configuration here
  devServer: {
    host: '0.0.0.0', // Listen on all network interfaces
    port: 8080      // Specify the port (optional if 8080 is the default)
  },

  // Keep your existing webpack configuration
  configureWebpack: {
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src')
      }
    }
  }
};