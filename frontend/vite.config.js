import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  plugins: [vue()],
  // Relative asset URLs support both / and a proxy-stripped /admin/ deployment.
  base: './',
  server: { proxy: { '/api': 'http://127.0.0.1:3080' } },
  build: { outDir: '../internal/webassets/static/dist', emptyOutDir: true, chunkSizeWarningLimit: 900 }
})
