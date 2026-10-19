import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Saat dev, /api diteruskan ke server Go agar cookie sesi tetap satu origin.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: false },
    },
  },
  build: {
    outDir: 'dist',
  },
  test: {
    environment: 'jsdom',
  },
});
