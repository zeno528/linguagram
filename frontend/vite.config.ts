import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      // Forward /api and /health to the Go backend on :8080.
      // Large repos can take a while to process — 5 min timeout avoids
      // premature ECONNRESET/flush errors on the proxy pipe.
      '/api': { target: 'http://localhost:8080', timeout: 5 * 60 * 1000 },
      '/health': 'http://localhost:8080',
    },
  },
})