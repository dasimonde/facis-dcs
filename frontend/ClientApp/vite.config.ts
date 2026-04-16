import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'url'
import { defineConfig, loadEnv, type Plugin } from 'vite'

// https://vite.dev/config/
export default defineConfig(({ mode, command }) => {
  const env = loadEnv(mode, process.cwd(), 'DCS_')
  const basePath = env.DCS_UI_PATH || '/ui/'

  
  // Plugin to inject base href in dev mode
  const baseHrefPlugin: Plugin = {
    name: 'base-href-inject',
    transformIndexHtml: {
      order: 'pre',
      handler(html) {
        if (command === 'serve') {
          // In dev mode, replace the placeholder with the actual base path
          return html.replace('__DCS_UI_BASE_PATH__', basePath)
        }
        // In build mode, leave the placeholder for inject-config.sh to handle
        return html
      }
    }
  }

  return {
    // during build, use relative paths such that we respect <base href>
    base: command === 'build' ? './' : basePath,
    plugins: [baseHrefPlugin, vue(), tailwindcss()],
    envPrefix: 'DCS_',
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src/', import.meta.url)),
        '@core': fileURLToPath(new URL('./src/core/', import.meta.url)),
        '@template-repository': fileURLToPath(new URL('./src/modules/template-repository/', import.meta.url)),
      },
    },
    server: {
      proxy: {
        '/api': {
          target: env.DCS_API_TARGET || 'http://localhost:8991',
          changeOrigin: true
        },
      },
    },
  }
})
