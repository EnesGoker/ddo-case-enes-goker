import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Geliştirmede /api ve /health istekleri yerel API'ye yönlendirilir (üretimde nginx yapar).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
})
