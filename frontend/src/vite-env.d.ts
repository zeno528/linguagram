/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Set to '1' by `build.ps1 -Public` to hide the path-entry UI in public builds. */
  readonly VITE_DISABLE_PATH?: string
}
