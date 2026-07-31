import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const rootDir = path.resolve(__dirname, '../../..')
const huskyPath = path.join(rootDir, 'node_modules/husky')

if (!fs.existsSync(huskyPath)) {
  console.error(
    '\x1b[31m%s\x1b[0m',
    "Error: Root dependencies are missing! Please run 'npm install' in the root directory first to enable Husky.",
  )
  process.exit(1)
}
