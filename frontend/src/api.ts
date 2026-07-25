import type { PublicRepoList, ScanResponse, UploadFile } from './types'

const API_BASE = '/api'

// Analyze a folder the user dragged into the browser. The frontend reads each
// file's head and posts a manifest here; the backend runs the same go-enry as
// the (now-removed) path mode used to, so results stay consistent. gitignore is
// the root .gitignore content (if any), forwarded so the backend can drop
// gitignored files and match GitHub's git-tracked-file scope. `signal` lets the
// UI abort the upload mid-flight via AbortController.
export async function scanFiles(
  projectName: string,
  files: UploadFile[],
  gitignore = '',
  opts: { signal?: AbortSignal } = {},
): Promise<ScanResponse> {
  const r = await fetch(`${API_BASE}/scan-files`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ projectName, files, gitignore }),
    signal: opts.signal,
  })
  if (!r.ok) {
    const err = await r.json().catch(() => ({ error: r.statusText }))
    throw new Error(err.error || `HTTP ${r.status}`)
  }
  return r.json()
}

// Analyze a public GitHub repo by URL. The backend downloads the repo's
// tarball (git-tracked files only) and runs the same go-enry. `signal` lets
// the UI abort via the shared AbortController, same as scanFiles.
export async function scanGitHub(
  url: string,
  opts: { signal?: AbortSignal } = {},
): Promise<ScanResponse> {
  const r = await fetch(`${API_BASE}/scan-github`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
    signal: opts.signal,
  })
  if (!r.ok) {
    const err = await r.json().catch(() => ({ error: r.statusText }))
    throw new Error(err.error || `HTTP ${r.status}`)
  }
  return r.json()
}

export async function listGitHubProfileRepos(
  url: string,
  opts: { signal?: AbortSignal } = {},
): Promise<PublicRepoList> {
  const r = await fetch(`${API_BASE}/github-profile-repos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
    signal: opts.signal,
  })
  if (!r.ok) {
    const err = await r.json().catch(() => ({ error: r.statusText }))
    throw new Error(err.error || `HTTP ${r.status}`)
  }
  return r.json()
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}
