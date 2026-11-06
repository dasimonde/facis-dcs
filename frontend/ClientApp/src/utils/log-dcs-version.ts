import type { DCSBuildRevision, DCSVersionInfo } from '@/config'

const badge = 'background:#2563eb;color:#fff;padding:1px 6px;border-radius:3px;font-weight:600;font-size:11px'
const kind = 'color:#94a3b8'
const sha = 'color:#38bdf8;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-weight:600'
const meta = 'color:#94a3b8;font-family:ui-monospace,SFMono-Regular,Menlo,monospace'

function line(name: string, rev: DCSBuildRevision): void {
  const built = rev.built_at ? `  ${rev.built_at}` : ''
  console.log(`%cDCS%c  ${name.padEnd(8)}  %c${rev.git_sha}%c${built}`, badge, kind, sha, meta)
}

export function logDcsVersion(info: DCSVersionInfo): void {
  line('frontend', info.frontend)
  if (info.backend) {
    line('backend', info.backend)
  }
}
