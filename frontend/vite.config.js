import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 3000,
    proxy: {
      '/upload': 'http://localhost:8084',
      '/status': 'http://localhost:8084',
      '/jobs': 'http://localhost:8084',
    }
  }
})
