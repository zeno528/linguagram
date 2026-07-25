export interface LanguageStat {
  name: string
  percentage: number
  color: string
  version?: string
  bytes: number
}

export interface ScanResponse {
  languages: LanguageStat[]
  totalBytes: number
  projectName: string
  githubUrl?: string
}

export interface PublicRepo {
  name: string
  fullName: string
  description: string
  url: string
  language?: string
  stars: number
  updatedAt: string
}

export interface PublicRepoList {
  owner: string
  repos: PublicRepo[]
}

// One file in a drag-and-drop upload manifest. Content is the first 16 KB,
// base64-encoded (empty ⇒ extension-only detection on the backend).
export interface UploadFile {
  path: string
  size: number
  content: string
}
