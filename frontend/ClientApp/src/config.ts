// Configuration: window.DCS_CONFIG (prod) or import.meta.env (dev)

export interface DCSConfig {
  API_BASE_URL: string
}

export interface DCSBuildRevision {
  git_sha: string
  built_at?: string
}

export interface DCSVersionInfo {
  frontend: DCSBuildRevision
  backend?: DCSBuildRevision
}

declare global {
  interface Window {
    DCS_CONFIG?: DCSConfig
    __DCS_VERSION__?: DCSVersionInfo
  }
}

export function getConfig(): DCSConfig {
  if (window.DCS_CONFIG) {
    return window.DCS_CONFIG
  }

  return {
    API_BASE_URL: import.meta.env.DCS_API_PATH || '/api',
  }
}

export function getUIBasePath(): string {
  const baseHref = document.querySelector('base')?.getAttribute('href')
  const fromBase = normalizeBasePath(baseHref)
  if (fromBase) {
    return fromBase
  }

  return normalizeBasePath(import.meta.env.DCS_UI_PATH) || '/ui/'
}

function normalizeBasePath(value?: string | null): string {
  if (!value) {
    return ''
  }

  const trimmed = value.trim()
  if (!trimmed) {
    return ''
  }

  let path = trimmed
  if (!path.startsWith('/')) {
    path = '/' + path
  }
  if (!path.endsWith('/')) {
    path = path + '/'
  }
  return path
}
