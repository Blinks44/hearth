import fs from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import sharp from 'sharp'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = path.resolve(root, '..')
const promoDir = path.join(repoRoot, '.local', 'promo')
const publicDir = path.join(root, 'public')
const screenshotsDir = path.join(publicDir, 'screenshots')
const logosDir = path.join(publicDir, 'logos')
const resourcesLogoDir = path.join(repoRoot, 'resources', 'icons', 'logo')

const LOGOS = [
  'nodedotjs.svg',
  'python.svg',
  'go.svg',
  'rust.svg',
  'rust-dark.svg',
  'bun.svg',
  'docker.svg',
  'kubernetes.svg',
  'postgresql.svg',
  'claude.svg',
  'openai.svg',
  'openai-dark.svg',
  'cursor.svg',
  'cursor-dark.svg',
  'githubcopilot.svg',
  'githubcopilot-dark.svg',
  'git.svg',
  'redis.svg',
  'mysql.svg',
  'mysql-dark.svg',
  'nginx.svg',
  'pnpm.svg',
  'pnpm-dark.svg',
]

const SCREENSHOTS = [
  'overview-light',
  'overview-dark',
  'projects-light',
  'projects-dark',
  'plan-diff-light',
  'plan-diff-dark',
  'docker-light',
  'docker-dark',
  'kubernetes-light',
  'kubernetes-dark',
  'ai-usage-light',
  'ai-usage-dark',
  'ai-mcp-light',
  'ai-mcp-dark',
  'cleanup-light',
  'cleanup-dark',
  'setup-light',
  'setup-dark',
]

await fs.mkdir(screenshotsDir, { recursive: true })
await fs.mkdir(logosDir, { recursive: true })

// App icon (192x192 PNG for crisp favicon & navbar mark)
const appIconSrc = path.join(repoRoot, 'resources', 'icon.png')
await sharp(appIconSrc)
  .resize(192, 192)
  .png({ compressionLevel: 9 })
  .toFile(path.join(publicDir, 'app-icon.png'))
console.log('Synced app-icon.png (192x192)')

// Copy brand logos
for (const file of LOGOS) {
  await fs.copyFile(path.join(resourcesLogoDir, file), path.join(logosDir, file))
}
console.log(`Copied ${LOGOS.length} brand SVGs to public/logos/`)

// Convert Retina promo screenshots (.local/promo/*.png) to optimized WebP
let converted = 0
for (const name of SCREENSHOTS) {
  const src = path.join(promoDir, `${name}.png`)
  try {
    await fs.access(src)
  } catch {
    console.warn(`Skipping missing screenshot: ${src}`)
    continue
  }
  const dest = path.join(screenshotsDir, `${name}.webp`)
  await sharp(src)
    .resize({ width: 2240, withoutEnlargement: true })
    .webp({ quality: 88, effort: 5 })
    .toFile(dest)
  const stat = await fs.stat(dest)
  converted++
  console.log(`Converted ${name}.webp (${Math.round(stat.size / 1024)} KB)`)
}

console.log(`Done: ${converted}/${SCREENSHOTS.length} screenshots converted.`)
