import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The Go binary embeds this directory (internal/spa).
const outDir = fileURLToPath(new URL('../internal/spa/dist', import.meta.url))

export default defineConfig({
  plugins: [
    react(),
    {
      // emptyOutDir wipes the tracked placeholder that keeps go:embed compiling.
      name: 'keep-embed-placeholder',
      closeBundle() {
        writeFileSync(`${outDir}/.gitkeep`, '')
      },
    },
  ],
  build: { outDir, emptyOutDir: true },
  server: { proxy: { '/api': 'http://localhost:8080' } },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
})
