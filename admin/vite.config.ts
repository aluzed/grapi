import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// base './': the server injects <base href> with admin.url, so one build
// works under any admin path.
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: {
    proxy: { '/admin/api': process.env.GRAPI_URL ?? 'http://localhost:1337' },
  },
})
