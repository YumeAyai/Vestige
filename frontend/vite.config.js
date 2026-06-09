import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import fs from 'node:fs'

function readConfig() {
  const path = process.env.NOUSMAIL_CONFIG || '../config.yaml'
  try {
    return fs.readFileSync(path, 'utf8')
  } catch {
    return ''
  }
}

function yamlValue(section, key, fallback) {
  const text = readConfig()
  const sectionMatch = text.match(new RegExp(`^${section}:\\n([\\s\\S]*?)(?=^[a-zA-Z_]+:|(?![\\s\\S]))`, 'm'))
  if (!sectionMatch) return fallback
  const lineMatch = sectionMatch[1].match(new RegExp(`^\\s+${key}:\\s*['"]?([^'"\\n]+)['"]?\\s*$`, 'm'))
  return lineMatch ? lineMatch[1].trim() : fallback
}

const devPort = Number(process.env.FRONTEND_DEV_PORT || yamlValue('frontend', 'dev_port', '5173'))
const apiProxy = process.env.FRONTEND_API_PROXY || yamlValue('frontend', 'api_proxy', 'http://localhost:8080')

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: '../local-backend/internal/webui/dist',
    emptyOutDir: true
  },
  server: {
    port: devPort,
    proxy: {
      '/api': apiProxy
    }
  }
})
