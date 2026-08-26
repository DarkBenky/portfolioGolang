import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), 'dist')
const HOST = process.env.SERVE_HOST || '0.0.0.0'
const PORT = Number(process.env.SERVE_PORT || 5173)

const DENY_SEGMENTS = new Set(['.env', '.git', '.aws', '.ssh', '.hg', '.svn', 'proc', 'etc', 'dev', 'sys', 'var'])
const DENY_EXTENSIONS = new Set(['.env', '.pem', '.key', '.crt', '.sqlite', '.sqlite3', '.db', '.gob', '.pt', '.h5', '.py', '.go'])

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.eot': 'application/vnd.ms-fontobject',
  '.txt': 'text/plain; charset=utf-8',
  '.wasm': 'application/wasm',
}

function resolveSafe(urlPath) {
  let decoded
  try {
    decoded = decodeURIComponent(urlPath)
  } catch {
    return null
  }
  if (decoded.includes('\0') || decoded.includes('..')) {
    return null
  }
  const relative = decoded.replace(/^\/+/, '')
  const resolved = path.resolve(ROOT, relative)
  if (resolved !== ROOT && !resolved.startsWith(ROOT + path.sep)) {
    return null
  }
  const segments = path.relative(ROOT, resolved).split(path.sep).filter(Boolean)
  for (const seg of segments) {
    if (DENY_SEGMENTS.has(seg) || seg.startsWith('.env') || seg.startsWith('.git')) {
      return null
    }
  }
  if (DENY_EXTENSIONS.has(path.extname(resolved).toLowerCase())) {
    return null
  }
  return resolved
}

function sendPlain(res, status, text, headers = {}) {
  res.writeHead(status, { 'Content-Type': 'text/plain; charset=utf-8', ...headers })
  res.end(text)
}

const baseHeaders = {
  'X-Content-Type-Options': 'nosniff',
  'X-Frame-Options': 'DENY',
  'Referrer-Policy': 'no-referrer',
}

const server = http.createServer((req, res) => {
  const urlPath = (req.url || '/').split('?')[0]
  const target = resolveSafe(urlPath)

  if (!target) {
    return sendPlain(res, 403, 'Forbidden', { ...baseHeaders, 'Cache-Control': 'no-store' })
  }

  fs.stat(target, (err, stat) => {
    if (err || !stat.isFile()) {
      const indexHtml = path.join(ROOT, 'index.html')
      return fs.readFile(indexHtml, (fErr, html) => {
        if (fErr) {
          return sendPlain(res, 404, 'Not found', { ...baseHeaders, 'Cache-Control': 'no-store' })
        }
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', ...baseHeaders, 'Cache-Control': 'no-cache' })
        res.end(html)
      })
    }
    const ext = path.extname(target).toLowerCase()
    const type = MIME[ext] || 'application/octet-stream'
    const cache = target.includes(path.sep + 'assets' + path.sep)
      ? 'public, max-age=31536000, immutable'
      : 'no-cache'
    res.writeHead(200, { 'Content-Type': type, ...baseHeaders, 'Cache-Control': cache })
    fs.createReadStream(target).pipe(res)
  })
})

server.listen(PORT, HOST, () => {
  console.log(`Serving ${ROOT} on http://${HOST}:${PORT}`)
  if (!fs.existsSync(path.join(ROOT, 'index.html'))) {
    console.warn('dist/index.html not found. Run "npm run build" first.')
  }
})
