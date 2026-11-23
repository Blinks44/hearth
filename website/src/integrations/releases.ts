import { createServerFn } from '@tanstack/react-start'
import { site } from '@/config/site'

export interface ReleaseAssets {
  version: string
  tag: string
  releaseUrl: string
  checksumsUrl: string
  macosDmg: string
  windowsAmd64: string
  windowsArm64: string
  linuxDebAmd64: string
  linuxDebArm64: string
  linuxTarAmd64: string
  linuxTarArm64: string
  isFallback: boolean
}

interface GitHubReleaseAsset {
  name?: string
  browser_download_url?: string
}

interface GitHubReleasePayload {
  tag_name?: string
  html_url?: string
  assets?: GitHubReleaseAsset[]
}

const CACHE_TTL_MS = 15 * 60 * 1000
const FETCH_TIMEOUT_MS = 3500

let cached: { value: ReleaseAssets; expiresAt: number } | null = null

export function fallbackReleaseAssets(): ReleaseAssets {
  const url = site.latestRelease
  return {
    version: site.defaultVersion,
    tag: `v${site.defaultVersion}`,
    releaseUrl: url,
    checksumsUrl: url,
    macosDmg: url,
    windowsAmd64: url,
    windowsArm64: url,
    linuxDebAmd64: url,
    linuxDebArm64: url,
    linuxTarAmd64: url,
    linuxTarArm64: url,
    isFallback: true,
  }
}

export function parseGitHubRelease(payload: GitHubReleasePayload): ReleaseAssets {
  const rawTag = payload.tag_name?.trim() || `v${site.defaultVersion}`
  const version = rawTag.replace(/^v/i, '') || site.defaultVersion
  const releaseUrl = payload.html_url?.trim() || site.latestRelease
  const assets = Array.isArray(payload.assets) ? payload.assets : []

  const findAsset = (predicate: (name: string) => boolean): string => {
    const match = assets.find((item) => item.name && item.browser_download_url && predicate(item.name.toLowerCase()))
    return match?.browser_download_url ?? releaseUrl
  }

  return {
    version,
    tag: rawTag,
    releaseUrl,
    checksumsUrl: findAsset((name) => name === 'sha256sums.txt' || name === 'sha256sums'),
    macosDmg: findAsset((name) => name.endsWith('.dmg')),
    windowsAmd64: findAsset((name) => name.endsWith('.exe') && (name.includes('amd64') || name.includes('x64'))),
    windowsArm64: findAsset((name) => name.endsWith('.exe') && name.includes('arm64')),
    linuxDebAmd64: findAsset((name) => name.endsWith('.deb') && (name.includes('amd64') || name.includes('x86_64'))),
    linuxDebArm64: findAsset((name) => name.endsWith('.deb') && name.includes('arm64')),
    linuxTarAmd64: findAsset((name) => name.endsWith('.tar.gz') && (name.includes('amd64') || name.includes('x86_64'))),
    linuxTarArm64: findAsset((name) => name.endsWith('.tar.gz') && name.includes('arm64')),
    isFallback: false,
  }
}

export async function fetchLatestRelease(
  fetcher: typeof fetch = fetch,
  now = Date.now(),
): Promise<ReleaseAssets> {
  if (cached && now < cached.expiresAt) {
    return cached.value
  }

  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS)

  try {
    const response = await fetcher(
      `https://api.github.com/repos/${site.repoOwner}/${site.repoName}/releases/latest`,
      {
        headers: {
          Accept: 'application/vnd.github+json',
          'User-Agent': 'hearth-website',
        },
        signal: controller.signal,
      },
    )
    if (!response.ok) {
      const fallback = fallbackReleaseAssets()
      cached = { value: fallback, expiresAt: now + 5 * 60 * 1000 }
      return fallback
    }
    const payload = (await response.json()) as GitHubReleasePayload
    const parsed = parseGitHubRelease(payload)
    cached = { value: parsed, expiresAt: now + CACHE_TTL_MS }
    return parsed
  } catch {
    const fallback = fallbackReleaseAssets()
    cached = { value: fallback, expiresAt: now + 2 * 60 * 1000 }
    return fallback
  } finally {
    clearTimeout(timer)
  }
}

export function resetReleaseCache(): void {
  cached = null
}

export const getLatestRelease = createServerFn({ method: 'GET' }).handler(async () => {
  return fetchLatestRelease()
})
