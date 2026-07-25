<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { motion, useReducedMotion } from 'motion-v'
import type { ECharts } from 'echarts/core'
import {
  siAngular,
  siAstro,
  siCoffeescript,
  siDjango,
  siDocusaurus,
  siElm,
  siExpress,
  siFastapi,
  siFastify,
  siFlask,
  siGatsby,
  siGo,
  siHono,
  siJavascript,
  siKoa,
  siLess,
  siMdx,
  siNextdotjs,
  siNestjs,
  siNodedotjs,
  siNuxt,
  siOpenjdk,
  siPreact,
  siPug,
  siPython,
  siQwik,
  siReact,
  siRemix,
  siRescript,
  siSass,
  siSolid,
  siSpringboot,
  siStorybook,
  siSvelte,
  siStylus,
  siTailwindcss,
  siTypescript,
  siVite,
  siVitepress,
  siVuedotjs,
} from 'simple-icons'
import type { SimpleIcon } from 'simple-icons'
import { scanFiles, scanGitHub, listGitHubProfileRepos, formatBytes } from './api'
import type { PublicRepo, ScanResponse, UploadFile } from './types'
import AppButton from './components/AppButton.vue'
import AppProgress from './components/AppProgress.vue'
import ToastHost from './components/ToastHost.vue'
import { useToast } from './composables/useToast'

const toast = useToast()

type Theme = 'light' | 'dark'

function initialTheme(): Theme {
  const saved = window.localStorage.getItem('theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

const theme = ref<Theme>(initialTheme())
const copyrightYear = new Date().getFullYear()
const themeToggleLabel = computed(() =>
  theme.value === 'dark' ? '切换至浅色主题' : '切换至深色主题',
)

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
}

// Drop mode is the only entry now. The drop flow has three cancellable phases:
// (1) collectFiles traversal, (2) readFileHead loop, (3) scanFiles POST.
const isBusy = ref(false)
const progress = ref(0)
const progressText = ref('')

const dragOver = ref(false)

// Shared mutable abort token — a plain object survives the awaits inside
// handleDrop's loops without forcing us to pass refs through every helper.
let cancelToken = { aborted: false }
let currentController: AbortController | null = null

const result = ref<ScanResponse | null>(null)
const errorMsg = ref('')
const githubUrl = ref('')
const profileUrl = ref('')
const profileRepos = ref<PublicRepo[]>([])
const profileOwner = ref('')
const profileQuery = ref('')
const isProfileLoading = ref(false)
const showImportPanel = ref(true)
const isProfilePickerOpen = ref(false)
const isProfileOwnerEditorOpen = ref(false)
type AnalysisSource = 'local' | 'repo' | 'profile' | null
const analysisSource = ref<AnalysisSource>(null)
const folderInput = ref<HTMLInputElement | null>(null)

// 三态卡片互斥渲染：progress / error / result。用单 section + key 直接切换，
// 避免退出动画未卸载旧节点时阻塞结果卡挂载。
const activeCard = computed<'progress' | 'error' | 'result' | null>(() => {
  if (isBusy.value) return 'progress'
  if (errorMsg.value) return 'error'
  if (result.value) return 'result'
  return null
})
const activeCardRef = ref<HTMLElement | null>(null)

type ChartKind = 'pie' | 'bar'

const chartKind = ref<ChartKind>('pie')
const chartRef = ref<HTMLDivElement | null>(null)
let chartInstance: ECharts | null = null
const selectedChartLanguage = ref<string | null>(null)

// echarts 按需 + 懒加载：首屏不加载图表库，首次 renderChart 时才动态 import 并
// 注册用到的组件（pie/bar/tooltip + CanvasRenderer）。首屏 bundle 从
// ~1.3MB 降到 ~300KB，echarts 独立 chunk 仅结果页加载，LCP 不再被 echarts 拖累。
let echartsPromise: Promise<typeof import('echarts/core')> | null = null
function loadEcharts() {
  if (!echartsPromise) {
    echartsPromise = (async () => {
      const echarts = await import('echarts/core')
      const [{ PieChart, BarChart }, { TooltipComponent, GridComponent }, { CanvasRenderer }] =
        await Promise.all([
          import('echarts/charts'),
          import('echarts/components'),
          import('echarts/renderers'),
        ])
      echarts.use([PieChart, BarChart, TooltipComponent, GridComponent, CanvasRenderer])
      return echarts
    })()
  }
  return echartsPromise
}

// ---- shared motion config ----
// §4: critically-damped spring by default (bounce 0, no overshoot). §14:
// under prefers-reduced-motion, collapse enter/exit to opacity-only so the
// motion stays non-vestibular. bounce/duration map to Apple's damping+response.
const reduce = useReducedMotion()
const spring = { type: 'spring' as const, bounce: 0, duration: 0.4 }
const pickerSpring = { type: 'spring' as const, bounce: 0, duration: 0.36 }
const cardAnimate = { opacity: 1, y: 0, scale: 1 }
const cardEnter = computed(() =>
  reduce.value ? { opacity: 0 } : { opacity: 0, y: 14, scale: 0.98 },
)
// Language-bar segments grow out from the left with a touch of bounce — they
// carry the momentum of the result arriving (§4 momentum ⇒ bounce allowed).
const segEnter = computed(() =>
  reduce.value ? { opacity: 0 } : { opacity: 0, scaleX: 0 },
)
// Table rows fade+offset in; opacity-only is safe on <tr> (transform on
// table-row is inconsistent across browsers).
const rowEnter = computed(() => (reduce.value ? { opacity: 0 } : { opacity: 0, y: 6 }))
const profilePickerInitial = computed(() =>
  reduce.value ? { opacity: 0 } : { height: 0, opacity: 0, y: -8 },
)
const profilePickerAnimate = computed(() => {
  if (reduce.value) return { opacity: isProfilePickerOpen.value ? 1 : 0 }
  return isProfilePickerOpen.value
    ? { height: 'auto', opacity: 1, y: 0 }
    : { height: 0, opacity: 0, y: -8 }
})
const profilePickerStyle = computed(() => ({
  marginBottom: isProfilePickerOpen.value ? '20px' : '0px',
  pointerEvents: isProfilePickerOpen.value ? 'auto' : 'none',
}))

// ---------- drop mode ----------
// Directories skipped before reading — mirrors enry's vendor heuristic so we
// never read (let alone upload) node_modules and friends.
const SKIP_DIRS = new Set([
  'node_modules', '.git', '.svn', '.hg', 'dist', 'build', '.next', '.nuxt',
  '.out', 'target', '__pycache__', '.venv', 'venv', 'env', '.idea', '.vscode',
  'bower_components', '.cache', '.turbo', '.gradle', 'coverage',
])

// These manifests declare the language and framework versions shown in the
// results table. Nested manifests are included for monorepos; they remain
// excluded from language byte totals on the backend.
const VERSION_MANIFESTS = new Set([
  'go.mod', 'package.json', '.nvmrc', 'pyproject.toml', 'pom.xml',
  'build.gradle', 'build.gradle.kts', 'requirements.txt',
])

// readEntries returns at most 100 entries per call — loop until it gives none.
function waitFor<T>(promise: Promise<T>, timeoutMs: number, message: string): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error(message)), timeoutMs)
    promise.then(
      (value) => { window.clearTimeout(timer); resolve(value) },
      (error) => { window.clearTimeout(timer); reject(error) },
    )
  })
}

function readEntries(reader: any): Promise<any[]> {
  return waitFor(new Promise((resolve, reject) => {
    const all: any[] = []
    const step = () => reader.readEntries((batch: any[]) => {
      if (!batch.length) return resolve(all)
      all.push(...batch)
      step()
    }, () => reject(new Error('读取文件夹失败，请确认该文件夹仍可访问')))
    step()
  }), 15_000, '读取文件夹超时，请尝试重新拖入')
}

interface CollectedFile {
  relPath: string
  getFile: () => Promise<File>
}

async function collectFiles(
  entry: any,
  prefix: string,
  out: CollectedFile[],
  isRoot = false,
) {
  if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
  if (entry.isFile) {
    out.push({ relPath: prefix + entry.name, getFile: () => getFile(entry) })
    return
  }
  if (!entry.isDirectory || SKIP_DIRS.has(entry.name)) return
  const children = await readEntries(entry.createReader())
  // Separate files from subdirectories: files go straight to out (no await
  // per file), directories are processed in parallel with bounded concurrency.
  // The old sequential for-await loop was O(N) per directory — for a flat
  // directory with 40K files this meant 40K sequential async ticks.
  const dirs: any[] = []
  const childPrefix = isRoot ? prefix : prefix + entry.name + '/'
  for (const child of children) {
    if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
    if (child.isFile) {
      out.push({ relPath: childPrefix + child.name, getFile: () => getFile(child) })
    } else if (child.isDirectory && !SKIP_DIRS.has(child.name)) {
      dirs.push(child)
    }
  }
  // Recurse into subdirectories in parallel (bounded at 16). runPool is
  // already defined below — each runner grabs the next dir from the queue.
  await runPool(dirs, 16, async (dir) => {
    await collectFiles(dir, childPrefix, out)
  })
}

// showDirectoryPicker 路径下的递归收集。返回的 CollectedFile[] 与 collectFiles 同形，输出直接喂
// 给 runFolderAnalysis。它取代 <input webkitdirectory> 走 File System Access API 是为了去掉
// Chromium v66+ 引入的「将 N 个文件上传到此站点？」安全确认弹窗（web 层无法关闭）——FS Access 走
// 「授予目录读权限」语义，不走 upload 路径，不触发数量警告。
async function collectFromDirectoryHandle(
  handle: FileSystemDirectoryHandle,
  prefix: string,
  out: CollectedFile[],
) {
  if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
  const dirs: FileSystemDirectoryHandle[] = []
  for await (const entry of handle.values()) {
    if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
    if (entry.kind === 'file') {
      out.push({ relPath: prefix + entry.name, getFile: () => entry.getFile() })
    } else if (entry.kind === 'directory' && !SKIP_DIRS.has(entry.name)) {
      dirs.push(entry)
    }
  }
  await runPool(dirs, 16, async (sub) => {
    await collectFromDirectoryHandle(sub, prefix + sub.name + '/', out)
  })
}

function getFile(entry: any): Promise<File> {
  return waitFor(new Promise((resolve, reject) => {
    entry.file((file: File) => resolve(file), () => reject(new Error(`无法读取文件：${entry.name}`)))
  }), 15_000, `读取文件超时：${entry.name}`)
}

// First 16 KB → base64. Chunked String.fromCharCode to avoid call-stack
// overflow on large binary heads (btoa itself needs a binary string).
async function readFileHead(file: File): Promise<string> {
  try {
    const bytes = new Uint8Array(await file.slice(0, 16 * 1024).arrayBuffer())
    let binary = ''
    for (let i = 0; i < bytes.length; i += 0x8000) {
      binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
    }
    return btoa(binary)
  } catch {
    return ''
  }
}

// runPool 并发执行 worker：concurrency 个 runner 抢 items 队列，每拿到一个调
// worker。把 readFileHead 阶段的串行 for-await 改成并发，几万文件提速数十倍。
// worker 抛错（如取消）会让 Promise.all reject，其余 runner 完成当前项后在下一
// 项开头检查 cancelToken 也会抛出，最终全部停止。
async function runPool<T>(items: T[], concurrency: number, worker: (item: T) => Promise<void>) {
  let i = 0
  const runners = Array.from({ length: Math.min(concurrency, items.length) }, async () => {
    while (i < items.length) {
      const idx = i++
      await worker(items[idx])
    }
  })
  await Promise.all(runners)
}

// cancelImport runs only while a drop is in progress. It owns the user-facing
// toast so the catch path in handleDrop doesn't double-fire one for the same
// abort — see `if (!cancelToken.aborted) toast.error(...)` below.
function cancelImport() {
  if (!isBusy.value) return
  cancelToken.aborted = true
  currentController?.abort()
  isBusy.value = false
  progress.value = 0
  progressText.value = ''
  result.value = null
  errorMsg.value = ''
  toast.info('已取消导入')
}

async function analyzeCollectedFiles(projectName: string, collected: CollectedFile[]) {
  if (!collected.length) throw new Error('没有读取到任何文件，请选择一个项目文件夹')

  const files: UploadFile[] = []
  let gitignoreContent = ''
  let done = 0
  let unreadable = 0
  let lastProgressAt = 0
  await runPool(collected, 64, async (item) => {
    if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
    try {
      const file = await item.getFile()
      if (item.relPath === '.gitignore' && !gitignoreContent) gitignoreContent = await file.text()
      const base = item.relPath.split('/').pop() || ''
      const needsContent = !base.includes('.') || base.indexOf('.') === 0 ||
        VERSION_MANIFESTS.has(base)
      files.push({ path: item.relPath, size: file.size, content: needsContent ? await readFileHead(file) : '' })
    } catch {
      unreadable++
    } finally {
      done++
    }
    const now = Date.now()
    if (now - lastProgressAt >= 60) {
      lastProgressAt = now
      progress.value = Math.round((done / collected.length) * 90)
      progressText.value = `正在读取文件… ${done}/${collected.length}`
    }
  })
  progress.value = Math.round((done / collected.length) * 90)
  progressText.value = `正在读取文件… ${done}/${collected.length}`
  if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
  if (!files.length) throw new Error('未能读取项目中的文件，请确认文件夹仍可访问后重试')

  progress.value = 95
  progressText.value = '正在分析语言占比…'
  result.value = await scanFiles(projectName || 'dropped-folder', files, gitignoreContent, { signal: currentController!.signal })
  if (!result.value.languages.length) throw new Error('未找到可统计的编程或标记语言文件')
  if (unreadable) toast.warning(`已跳过 ${unreadable} 个无法读取的文件`)
  if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
  progress.value = 100
}

async function runFolderAnalysis(projectName: string, collect: () => Promise<CollectedFile[]>) {
  analysisSource.value = 'local'
  cancelToken = { aborted: false }
  currentController = new AbortController()
  isBusy.value = true
  errorMsg.value = ''
  result.value = null
  progress.value = 0
  progressText.value = '正在读取文件…'
  try {
    await analyzeCollectedFiles(projectName, await collect())
  } catch (e: any) {
    if (!cancelToken.aborted) {
      errorMsg.value = e.message ?? String(e)
      toast.error(errorMsg.value)
    }
  } finally {
    isBusy.value = false
    setTimeout(() => { progress.value = 0; progressText.value = '' }, 600)
  }
}

async function handleDrop(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
  const items = e.dataTransfer?.items
  if (!items?.length) return
  if (typeof (items[0] as any).webkitGetAsEntry !== 'function') {
    toast.warning('当前浏览器不支持拖拽文件夹，请使用“选择文件夹”')
    return
  }
  const firstEntry = (items[0] as any).webkitGetAsEntry()
  const rootName = firstEntry?.name || 'dropped-folder'
  await runFolderAnalysis(rootName, async () => {
    const collected: CollectedFile[] = []
    for (const item of Array.from(items)) {
      const entry = (item as any).webkitGetAsEntry()
      if (!entry) continue
      await collectFiles(entry, '', collected, entry.isDirectory)
    }
    return collected
  })
}

async function openFolderPicker() {
  if (isBusy.value) return
  // 优先 File System Access API：取代 webkitdirectory 是为了去掉 Chromium v66+ 在大目录上的
  // 「将 N 个文件上传到此站点？」安全确认弹窗（web 层没有关闭开关）。showDirectoryPicker 走的是
  // 「授予目录读权限」语义，不走 upload 路径，浏览器不会再追加数量警告。
  const win = window as Window & {
    showDirectoryPicker?: (options?: { mode?: 'read' | 'readwrite' }) => Promise<FileSystemDirectoryHandle>
  }
  if (win.showDirectoryPicker) {
    try {
      const dirHandle = await win.showDirectoryPicker({ mode: 'read' })
      await runFolderAnalysis(dirHandle.name, async () => {
        const out: CollectedFile[] = []
        await collectFromDirectoryHandle(dirHandle, '', out)
        return out
      })
    } catch (e: any) {
      // 用户在系统目录选择器里取消 → AbortError，安静吞掉
      if (e?.name === 'AbortError') return
      // 其它异常（罕见，比如不安全上下文或策略拒绝）：退回 webkitdirectory 兜底
      folderInput.value?.click()
    }
    return
  }
  // Firefox / Safari / 移动端：showDirectoryPicker 不支持，退回 webkitdirectory——那个 N 个
  // 文件警告在非 Chromium 上不存在，但若用户使用了改造过的 Chromium 仍可能见到。
  folderInput.value?.click()
}

function shouldSkipSelectedFile(relPath: string): boolean {
  return relPath.split('/').slice(0, -1).some((segment) => SKIP_DIRS.has(segment))
}

async function handleFolderSelection(e: Event) {
  const selected = Array.from((e.target as HTMLInputElement).files || [])
  ;(e.target as HTMLInputElement).value = ''
  if (!selected.length) return
  const firstPath = selected[0].webkitRelativePath || selected[0].name
  const [rootName] = firstPath.split('/')
  await runFolderAnalysis(rootName, async () => selected.flatMap((file) => {
    const path = file.webkitRelativePath || file.name
    const segments = path.split('/')
    const relPath = segments.length > 1 ? segments.slice(1).join('/') : path
    return shouldSkipSelectedFile(relPath) ? [] : [{ relPath, getFile: () => Promise.resolve(file) }]
  }))
}

// ---------- github url mode ----------
// 复用 drop 模式的 isBusy/progress/result 状态与 cancel 机制，只换数据源：
// 后端下载 tarball 后用同一套 go-enry 分类，结果展示完全一致。
async function analyzeGithub(source: Exclude<AnalysisSource, 'local' | null>) {
  const url = githubUrl.value.trim()
  if (!url) return

  analysisSource.value = source
  cancelToken = { aborted: false }
  currentController = new AbortController()

  isBusy.value = true
  errorMsg.value = ''
  result.value = null
  progress.value = 0
  progressText.value = '正在下载并分析仓库…'

  try {
    result.value = await scanGitHub(url, { signal: currentController.signal })
    if (cancelToken.aborted) throw new DOMException('Aborted', 'AbortError')
    progress.value = 100
  } catch (e: any) {
    if (!cancelToken.aborted) {
      errorMsg.value = e.message ?? String(e)
      toast.error(errorMsg.value)
    }
  } finally {
    isBusy.value = false
    setTimeout(() => {
      progress.value = 0
      progressText.value = ''
    }, 600)
  }
}

// 从剪贴板读取 URL 粘进输入框。readText 在非安全上下文（http / 未聚焦）下会抛错
// 或不存在，统一兜底成 toast 提示，让用户改用手动粘贴。
async function pasteFromClipboard(target: 'repo' | 'profile' = 'repo') {
  if (!navigator.clipboard?.readText) {
    toast.warning('当前浏览器不支持读取剪贴板')
    return
  }
  try {
    const text = (await navigator.clipboard.readText()).trim()
    if (!text) {
      toast.info('剪贴板为空')
      return
    }
    if (target === 'profile') profileUrl.value = text
    else githubUrl.value = text
    toast.success('已粘贴')
  } catch {
    toast.error('读取剪贴板失败，请手动粘贴')
  }
}

// ---------- chart ----------
const topLang = computed(() => result.value?.languages[0])
const maxChartLanguages = 8
const languageQuery = ref('')
const filteredLanguages = computed(() => {
  if (!result.value) return []
  const query = languageQuery.value.trim().toLocaleLowerCase()
  if (!query) return result.value.languages
  return result.value.languages.filter((language) =>
    language.name.toLocaleLowerCase().includes(query),
  )
})

function technologyVersions(language: string, version?: string): string[] {
  if (!version) return []
  return version.split(' · ').map((item, index) =>
    // The first value belongs to the language row itself; framework entries
    // already carry their own name (for example, "React ^19"). A framework
    // can also be the only declaration on a row (for example, Tailwind on
    // CSS), in which case keeping it unprefixed lets its real brand icon show.
    index === 0 && !technologyIcon(item) ? `${language} ${item}` : item,
  )
}

// The backend produces display labels such as "TypeScript ~6" and
// "React ^19". Match their leading technology name to a locally bundled,
// brand-coloured Simple Icon; unfamiliar technologies retain the same chip
// without an icon instead of falling back to a misleading glyph.
const technologyIcons: Record<string, SimpleIcon> = {
  'TypeScript': siTypescript,
  'JavaScript': siJavascript,
  'Node': siNodedotjs,
  'JavaScript Node': siNodedotjs,
  'Vue': siVuedotjs,
  'Astro': siAstro,
  'Svelte': siSvelte,
  'CoffeeScript': siCoffeescript,
  'Less': siLess,
  'Sass': siSass,
  'SCSS': siSass,
  'Stylus': siStylus,
  'Pug': siPug,
  'MDX': siMdx,
  'Elm': siElm,
  'ReScript': siRescript,
  'React': siReact,
  'Next.js': siNextdotjs,
  'Angular': siAngular,
  'Nuxt': siNuxt,
  'Preact': siPreact,
  'SolidJS': siSolid,
  'Qwik': siQwik,
  'Remix': siRemix,
  'Gatsby': siGatsby,
  'Express': siExpress,
  'NestJS': siNestjs,
  'Fastify': siFastify,
  'Koa': siKoa,
  'Hono': siHono,
  'Vite': siVite,
  'Tailwind CSS': siTailwindcss,
  'VitePress': siVitepress,
  'Docusaurus': siDocusaurus,
  'Storybook': siStorybook,
  'FastAPI': siFastapi,
  'Django': siDjango,
  'Flask': siFlask,
  'Spring Boot': siSpringboot,
  'Go': siGo,
  'Python': siPython,
  'Java': siOpenjdk,
}

function technologyIcon(label: string): SimpleIcon | null {
  const iconName = Object.keys(technologyIcons)
    .sort((a, b) => b.length - a.length)
    .find((name) => label === name || label.startsWith(`${name} `))
  return iconName ? technologyIcons[iconName] : null
}

function cssVar(name: string, fallback: string): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

const filteredProfileRepos = computed(() => {
  const query = profileQuery.value.trim().toLocaleLowerCase()
  if (!query) return profileRepos.value
  return profileRepos.value.filter((repo) =>
    `${repo.name} ${repo.description} ${repo.language ?? ''}`.toLocaleLowerCase().includes(query),
  )
})

async function loadProfileRepos() {
  const url = profileUrl.value.trim()
  if (!url || isProfileLoading.value) return
  isProfileLoading.value = true
  isProfilePickerOpen.value = false
  try {
    const data = await listGitHubProfileRepos(url)
    profileOwner.value = data.owner
    profileRepos.value = data.repos
    profileQuery.value = ''
    isProfileOwnerEditorOpen.value = false
    isProfilePickerOpen.value = true
    if (!data.repos.length) toast.info('没有找到符合条件的公开仓库')
  } catch (e: any) {
    toast.error(e.message ?? String(e))
  } finally {
    isProfileLoading.value = false
  }
}

function analyzeProfileRepo(repo: PublicRepo) {
  githubUrl.value = repo.url
  isProfilePickerOpen.value = false
  void analyzeGithub('profile')
}

function openImportPanel() {
  showImportPanel.value = true
  isProfilePickerOpen.value = false
}

function toggleProfilePicker() {
  isProfileOwnerEditorOpen.value = false
  isProfilePickerOpen.value = !isProfilePickerOpen.value
}

function changeProfileOwner() {
  profileUrl.value = ''
  isProfilePickerOpen.value = false
  isProfileOwnerEditorOpen.value = true
}

function cancelProfileOwnerChange() {
  isProfileOwnerEditorOpen.value = false
}

function chartDataFor(
  languages: ScanResponse['languages'],
  otherColor: string,
  selectedName: string | null = null,
) {
  const data = languages.map((l) => ({
    name: l.name,
    value: Number(l.percentage.toFixed(2)),
    bytes: l.bytes,
    itemStyle: { color: l.color },
  }))
  if (data.length <= maxChartLanguages) return data

  const selectedIndex = data.findIndex((item) => item.name === selectedName)
  // A language from the long tail should still become visible when selected
  // from the detail list. Keep the chart compact by replacing only the last
  // dominant slot, then aggregate every unshown language into "其他".
  const visibleIndices = selectedIndex >= maxChartLanguages
    ? [...Array.from({ length: maxChartLanguages - 1 }, (_, index) => index), selectedIndex]
    : Array.from({ length: maxChartLanguages }, (_, index) => index)
  const visibleIndexSet = new Set(visibleIndices)
  const visible = visibleIndices.map((index) => data[index])
  const remaining = data.filter((_, index) => !visibleIndexSet.has(index))
  return [
    ...visible,
    {
      name: `其他（${remaining.length} 种）`,
      value: Number(remaining.reduce((sum, item) => sum + item.value, 0).toFixed(2)),
      bytes: remaining.reduce((sum, item) => sum + item.bytes, 0),
      itemStyle: { color: otherColor },
    },
  ]
}

const chartLegend = computed(() => {
  // Make the "other" swatch track the active theme's muted color.
  theme.value
  if (!result.value) return []
  return chartDataFor(result.value.languages, cssVar('--muted', '#6e6e73'), selectedChartLanguage.value)
})

function toggleChartLanguage(name: string) {
  selectedChartLanguage.value = selectedChartLanguage.value === name ? null : name
}

function handleChartElementClick(params: any) {
  if (params.componentType !== 'series' || !params.name) return
  toggleChartLanguage(String(params.name))
}

function handleChartBlankClick(event: any) {
  // ECharts item events only fire on a series element. ZRender receives every
  // canvas click, so a missing target is the reliable blank-area reset.
  if (!event.target) selectedChartLanguage.value = null
}

function syncChartFocus(chart: ECharts, data: ReturnType<typeof chartDataFor>) {
  // ECharts keeps emphasis state independently from the option data. Clear it
  // before every sync so toggling a language off cannot leave a slice/bar in
  // its lifted highlight state.
  chart.dispatchAction({ type: 'downplay', seriesIndex: 0 })
  chart.dispatchAction({ type: 'hideTip' })

  if (!selectedChartLanguage.value) return
  const dataIndex = data.findIndex((item) => item.name === selectedChartLanguage.value)
  if (dataIndex < 0) return
  chart.dispatchAction({ type: 'highlight', seriesIndex: 0, dataIndex })
  chart.dispatchAction({ type: 'showTip', seriesIndex: 0, dataIndex })
}

async function renderChart() {
  if (!chartRef.value || !result.value || !result.value.languages.length) return

  // The result card is mounted by a keyed transition. Wait for Vue to flush
  // its layout before ECharts reads the canvas dimensions.
  await nextTick()
  const echarts = await loadEcharts()
  // await 期间结果可能被清空（取消/新分析），重新校验后再渲染。
  if (!chartRef.value || !result.value || !result.value.languages.length) return

  // Read the body's CSS custom properties so ECharts text matches the DOM
  // without hard-coding color values here — style.css stays the single source.
  const font = cssVar('--font-sans', 'system-ui, sans-serif')
  const fg = cssVar('--fg', '#1d1d1f')
  const tooltipBg = cssVar('--tooltip-bg', 'rgba(255, 255, 255, 0.82)')
  const pieSeam = cssVar('--pie-seam', '#fff')
  const muted = cssVar('--muted', '#6e6e73')
  const hairline = cssVar('--hairline', 'rgba(0, 0, 0, 0.08)')
  const chartGridline = cssVar('--chart-gridline', 'rgba(0, 0, 0, 0.045)')

  // The result card unmounts while busy (we clear `result` between analyses),
  // so chartRef may point at a fresh DOM node on the 2nd+ analysis. Bind to the
  // current node — reuse its instance if it has one, otherwise dispose the
  // stale singleton and init fresh. Without this the singleton stays glued to
  // the removed node and the pie silently stops rendering until a full reload.
  let chart = echarts.getInstanceByDom(chartRef.value)
  if (!chart) {
    if (chartInstance) chartInstance.dispose()
    chart = echarts.init(chartRef.value)
  }
  chartInstance = chart
  // Re-rendering happens whenever a language is selected, so remove these
  // exact handlers before re-attaching them to prevent duplicate toggles.
  chart.off('click', handleChartElementClick)
  chart.getZr().off('click', handleChartBlankClick)
  chart.on('click', handleChartElementClick)
  chart.getZr().on('click', handleChartBlankClick)
  const langs = result.value.languages
  // Fixed-size charts cannot express a long tail legibly. Keep the dominant
  // languages visible and aggregate the remainder; the adjacent table still
  // exposes every language independently.
  const data = chartDataFor(langs, muted, selectedChartLanguage.value)
  const displayData = selectedChartLanguage.value
    ? data.map((item) => ({
        ...item,
        itemStyle: { ...item.itemStyle, opacity: item.name === selectedChartLanguage.value ? 1 : 0.28 },
      }))
    : data
  const tooltip = {
    trigger: 'item' as const,
    // Translucent tooltip to match the glass cards (§12 material). ECharts
    // renders the tooltip as its own DOM, so extraCssText carries the blur.
    backgroundColor: tooltipBg,
    borderColor: 'transparent',
    borderWidth: 0,
    extraCssText:
      '-webkit-backdrop-filter: blur(20px) saturate(180%);' +
      'backdrop-filter: blur(20px) saturate(180%);' +
      'border-radius: 10px; box-shadow: 0 4px 16px rgba(0,0,0,0.12);',
    textStyle: { fontFamily: font, color: fg, fontSize: 12 },
    formatter: (p: any) =>
      `${p.marker} ${p.name}<br/><b>${p.value}%</b> · ${formatBytes(p.data?.bytes ?? 0)}`,
  }

  if (chartKind.value === 'bar') {
    const descending = [...displayData].reverse()
    chart.setOption({
      tooltip,
      grid: { top: 16, right: 44, bottom: 18, left: 12, containLabel: true },
      xAxis: {
        type: 'value', max: 100,
        axisLabel: { color: muted, formatter: '{value}%' },
        axisLine: { lineStyle: { color: hairline } },
        splitLine: { lineStyle: { color: chartGridline } },
      },
      yAxis: {
        type: 'category', data: descending.map((l) => l.name),
        axisTick: { show: false }, axisLine: { show: false },
        axisLabel: { color: muted, width: 88, overflow: 'truncate' },
      },
      series: [{
        type: 'bar', data: descending, barMaxWidth: 24,
        itemStyle: { borderRadius: [0, 5, 5, 0] },
        label: { show: true, position: 'right', color: muted, formatter: '{c}%' },
        emphasis: { itemStyle: { shadowBlur: 12, shadowColor: 'rgba(0,0,0,0.16)' } },
      }],
    }, { notMerge: true })
    syncChartFocus(chart, descending)
    return
  }

  chart.setOption({
    tooltip,
    series: [{
      type: 'pie',
      // The adjacent language table is the chart legend. Keeping a second,
      // scrollable legend inside this fixed-height canvas makes its rows run
      // into the pie as a project gains languages.
      radius: ['0%', '62%'],
      center: ['50%', '50%'],
      avoidLabelOverlap: true,
      // White seams separate slices against the solid pie-box face (same role
      // as GitHub's separators, retained for readability).
      itemStyle: { borderColor: pieSeam, borderWidth: 2, borderRadius: 4 },
      label: { show: false },
      labelLine: { show: false },
      emphasis: {
        scale: true,
        scaleSize: 8,
        itemStyle: { shadowBlur: 16, shadowColor: 'rgba(0,0,0,0.16)' },
      },
      animationType: 'expansion',
      animationEasing: 'cubicOut',
      data: displayData,
    }],
  }, { notMerge: true })
  syncChartFocus(chart, displayData)
}

// 同时监听结果、画布和图表类型；key 切换后 chartRef 会指向新结果卡中的节点，
// flush:'post' 保证 DOM 已挂载再初始化 ECharts。
watch(() => result.value, () => {
  selectedChartLanguage.value = null
  languageQuery.value = ''
})
watch(activeCard, async (card) => {
  if (card !== 'result') return
  showImportPanel.value = false
  isProfilePickerOpen.value = false
  isProfileOwnerEditorOpen.value = false
  // Wait until the progress card has been replaced by the mounted result card
  // before scrolling; doing this at fetch completion would target the old card.
  await nextTick()
  activeCardRef.value?.scrollIntoView({
    behavior: reduce.value ? 'auto' : 'smooth',
    block: 'start',
  })
})
watch([() => result.value, chartRef, chartKind, selectedChartLanguage], renderChart, { flush: 'post' })

watch(theme, (value) => {
  document.documentElement.dataset.theme = value
  window.localStorage.setItem('theme', value)
  void renderChart()
}, { immediate: true })

const onResize = () => chartInstance?.resize()
onMounted(() => window.addEventListener('resize', onResize))
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  chartInstance?.dispose()
  chartInstance = null
})
</script>

<template>
  <header
    v-motion
    :initial="reduce ? { opacity: 0 } : { opacity: 0, y: -10 }"
    :animate="{ opacity: 1, y: 0 }"
    :transition="spring"
  >
    <div class="header-row">
      <div class="header-text">
        <div class="header-title">
          <div class="brand-lockup">
            <img src="/favicon.svg?v=2" class="logo" alt="" width="36" height="36" aria-hidden="true" />
            <h1>Linguagram</h1>
            <a
              class="gh-link"
              href="https://github.com/zeno528/linguagram"
              target="_blank"
              rel="noopener noreferrer"
              aria-label="GitHub 仓库源码"
              title="GitHub 仓库源码"
            >
              <svg viewBox="0 0 16 16" width="20" height="20" aria-hidden="true" focusable="false">
                <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
              </svg>
            </a>
          </div>
          <p>分析本地项目、GitHub 仓库与作者公开项目的语言构成</p>
        </div>
      </div>
      <div class="header-actions">
        <button
          class="theme-toggle"
          type="button"
          :aria-label="themeToggleLabel"
          :aria-pressed="theme === 'dark'"
          :title="themeToggleLabel"
          @click="toggleTheme"
        >
          <svg v-if="theme === 'dark'" viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="12" cy="12" r="4" />
            <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
          </svg>
          <svg v-else viewBox="0 0 24 24" aria-hidden="true">
            <path d="M20.2 14.2A8.5 8.5 0 0 1 9.8 3.8 8.5 8.5 0 1 0 20.2 14.2Z" />
          </svg>
        </button>
      </div>
    </div>
  </header>

  <section v-if="showImportPanel || !result" class="scan-card">
    <div
      class="dropzone"
      :class="{ over: dragOver, disabled: isBusy }"
      @dragenter.prevent="dragOver = true"
      @dragover.prevent="dragOver = true"
      @dragleave.prevent="dragOver = false"
      @drop="handleDrop"
    >
      <div class="dz-icon">📁</div>
      <p class="dz-title">{{ isBusy ? progressText : '把项目文件夹拖到这里' }}</p>
      <p class="dz-hint">拖拽无响应时，可改用下方选择文件夹</p>
      <button class="folder-picker" type="button" :disabled="isBusy" @click="openFolderPicker">选择文件夹</button>
    </div>
    <input
      id="folder-input"
      ref="folderInput"
      class="folder-input"
      name="project-folder"
      type="file"
      webkitdirectory
      multiple
      @change="handleFolderSelection"
    />

    <div class="gh-input">
      <div class="gh-field" :class="{ disabled: isBusy }">
        <input
          v-model="githubUrl"
          type="url"
          placeholder="或粘贴 GitHub 公开仓库地址，如 https://github.com/owner/repo"
          :disabled="isBusy"
          @keyup.enter="() => analyzeGithub('repo')"
        />
        <button
          class="gh-icon-btn"
          type="button"
          aria-label="粘贴"
          :disabled="isBusy"
          @click="() => pasteFromClipboard()"
        >
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
            <rect x="8" y="2" width="8" height="4" rx="1" />
          </svg>
        </button>
        <button
          v-if="githubUrl"
          class="gh-icon-btn"
          type="button"
          aria-label="清空"
          :disabled="isBusy"
          @click="githubUrl = ''"
        >
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
            <path d="M18 6 6 18M6 6l12 12" />
          </svg>
        </button>
      </div>
      <AppButton variant="plain" :disabled="isBusy || !githubUrl.trim()" @click="() => analyzeGithub('repo')">
        <svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9M10 19V5M16 19v-7M22 19H2" /></svg>
        分析
      </AppButton>
    </div>

    <div class="profile-picker">
      <div class="profile-picker-heading">
        <strong>从作者主页选择仓库</strong>
        <span>仅展示本人公开、未归档且非 Fork 的仓库</span>
      </div>
      <div class="gh-input profile-input">
        <div class="gh-field" :class="{ disabled: isProfileLoading }">
          <input
            v-model="profileUrl"
            type="url"
            placeholder="GitHub 作者主页，如 https://github.com/zeno528"
            :disabled="isProfileLoading"
            @keyup.enter="loadProfileRepos"
          />
          <button
            class="gh-icon-btn"
            type="button"
            aria-label="粘贴作者主页"
            :disabled="isProfileLoading"
            @click="pasteFromClipboard('profile')"
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
              <rect x="8" y="2" width="8" height="4" rx="1" />
            </svg>
          </button>
          <button
            v-if="profileUrl"
            class="gh-icon-btn"
            type="button"
            aria-label="清空作者主页"
            :disabled="isProfileLoading"
            @click="profileUrl = ''"
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
              <path d="M18 6 6 18M6 6l12 12" />
            </svg>
          </button>
        </div>
        <AppButton variant="plain" :disabled="isProfileLoading || !profileUrl.trim()" @click="loadProfileRepos">
          <template v-if="isProfileLoading">读取中…</template>
          <template v-else>
            <svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9M10 19V5M16 19v-7M22 19H2" /></svg>
            分析
          </template>
        </AppButton>
      </div>
    </div>
  </section>

  <div v-else class="analysis-toolbar">
    <span>当前查看 {{ result?.projectName }} 的分析结果</span>
    <div class="analysis-toolbar-actions">
      <template v-if="analysisSource === 'profile' && profileOwner && !isProfileOwnerEditorOpen">
        <AppButton variant="plain" @click="toggleProfilePicker">切换仓库</AppButton>
        <AppButton variant="plain" @click="changeProfileOwner">更换作者</AppButton>
      </template>
      <AppButton v-else-if="isProfileOwnerEditorOpen" variant="plain" @click="cancelProfileOwnerChange">取消</AppButton>
      <AppButton variant="plain" @click="openImportPanel">新建分析</AppButton>
    </div>
  </div>

  <section v-if="analysisSource === 'profile' && isProfileOwnerEditorOpen" class="profile-switcher" aria-label="更换 GitHub 作者">
    <div>
      <strong>更换作者</strong>
      <p>输入另一位作者的 GitHub 主页，仓库列表会直接在此处更新。</p>
    </div>
    <div class="gh-input profile-input">
      <div class="gh-field" :class="{ disabled: isProfileLoading }">
        <input
          v-model="profileUrl"
          type="url"
          placeholder="GitHub 作者主页，如 https://github.com/zeno528"
          :disabled="isProfileLoading"
          @keyup.enter="loadProfileRepos"
        />
        <button class="gh-icon-btn" type="button" aria-label="粘贴新作者主页" :disabled="isProfileLoading" @click="pasteFromClipboard('profile')">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" /><rect x="8" y="2" width="8" height="4" rx="1" /></svg>
        </button>
        <button v-if="profileUrl" class="gh-icon-btn" type="button" aria-label="清空新作者主页" :disabled="isProfileLoading" @click="profileUrl = ''">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12" /></svg>
        </button>
      </div>
      <AppButton variant="plain" :disabled="isProfileLoading || !profileUrl.trim()" @click="loadProfileRepos">
        <template v-if="isProfileLoading">读取中…</template>
        <template v-else>
          <svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9M10 19V5M16 19v-7M22 19H2" /></svg>
          分析
        </template>
      </AppButton>
    </div>
  </section>

  <motion.section
    v-if="profileOwner"
    layout
    class="profile-repo-list"
    :aria-label="`${profileOwner} 的公开仓库`"
    :aria-hidden="!isProfilePickerOpen"
    :inert="!isProfilePickerOpen"
    :initial="profilePickerInitial"
    :animate="profilePickerAnimate"
    :transition="pickerSpring"
    :style="profilePickerStyle"
  >
    <div class="profile-repo-list-header">
      <span>{{ profileOwner }} · {{ filteredProfileRepos.length }} / {{ profileRepos.length }} 个仓库</span>
      <input v-model="profileQuery" type="search" aria-label="搜索公开仓库" placeholder="搜索仓库" />
    </div>
    <div class="profile-repo-scroller">
      <button
        v-for="repo in filteredProfileRepos"
        :key="repo.fullName"
        class="profile-repo-item"
        type="button"
        :disabled="isBusy"
        :aria-label="`分析仓库 ${repo.fullName}`"
        @click="analyzeProfileRepo(repo)"
      >
        <div>
          <strong>{{ repo.name }}</strong>
          <p>{{ repo.description || '暂无描述' }}</p>
          <small>{{ repo.language || '未标注语言' }} · ★ {{ repo.stars }} · 更新于 {{ new Date(repo.updatedAt).toLocaleDateString() }}</small>
        </div>
        <svg class="profile-repo-arrow" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 18 6-6-6-6" /></svg>
      </button>
      <p v-if="!filteredProfileRepos.length" class="profile-repo-empty">没有匹配的仓库</p>
    </div>
  </motion.section>

  <motion.section
    v-if="activeCard"
    ref="activeCardRef"
    :key="activeCard"
    layout
    :initial="cardEnter"
    :animate="cardAnimate"
    :transition="spring"
    :class="activeCard === 'progress' ? 'progress-card' : 'result-card'"
  >
      <template v-if="activeCard === 'progress'">
        <AppProgress
        :percentage="progress"
        :indeterminate="false"
        :status="progress >= 100 ? 'success' : undefined"
        :show-text="true"
      />
      <p class="progress-text">{{ progressText }}</p>
      <div class="progress-actions">
        <AppButton variant="plain" @click="cancelImport">取消</AppButton>
      </div>
      </template>
      <p v-else-if="activeCard === 'error'" class="error">❌ {{ errorMsg }}</p>
      <template v-else-if="result">
      <div class="result-header">
        <div class="result-title-row">
          <h2>{{ result.projectName }}</h2>
          <a
            v-if="result.githubUrl"
            :href="result.githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="gh-repo-link"
            :title="result.githubUrl"
          >
            <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
            GitHub
          </a>
        </div>
        <span class="meta">
          共 {{ formatBytes(result.totalBytes) }} · {{ result.languages.length }} 种语言
          <span v-if="topLang"> · 主要 <strong>{{ topLang.name }}</strong> {{ topLang.percentage }}%</span>
        </span>
      </div>

      <!-- GitHub-style horizontal bar; segments grow out from the left, staggered -->
      <div class="lang-bar" :title="result.languages.map((l) => `${l.name} ${l.percentage}%`).join(' · ')">
        <div
          v-for="(l, i) in result.languages"
          :key="l.name"
          v-motion
          :initial="segEnter"
          :animate="{ opacity: 1, scaleX: 1 }"
          :transition="{ type: 'spring', bounce: 0.2, duration: 0.5, delay: i * 0.05 }"
          class="seg"
          :style="{ width: l.percentage + '%', background: l.color, transformOrigin: 'left' }"
        />
      </div>

      <div class="chart-toggle" role="group" aria-label="图表类型">
        <button type="button" :class="{ active: chartKind === 'pie' }" @click="chartKind = 'pie'">扇形图</button>
        <button type="button" :class="{ active: chartKind === 'bar' }" @click="chartKind = 'bar'">柱形图</button>
      </div>

      <div class="lang-grid">
        <div class="chart-panel">
          <div ref="chartRef" class="chart-box"></div>
          <div class="chart-legend" aria-label="图表语言占比">
            <button
              v-for="item in chartLegend"
              :key="item.name"
              type="button"
              :class="{ active: selectedChartLanguage === item.name }"
              :aria-pressed="selectedChartLanguage === item.name"
              @click="toggleChartLanguage(item.name)"
            >
              <span class="chart-legend-swatch" :style="{ background: item.itemStyle.color }"></span>
              <span>{{ item.name }}</span>
              <strong>{{ item.value.toFixed(2) }}%</strong>
            </button>
          </div>
        </div>
        <section class="language-panel" aria-label="语言明细">
          <div class="language-panel-header">
            <div>
              <h3>全部语言</h3>
              <p>{{ filteredLanguages.length }} / {{ result.languages.length }} 种</p>
            </div>
            <input
              v-model="languageQuery"
              class="language-search"
              type="search"
              aria-label="搜索语言"
              placeholder="搜索语言"
            />
          </div>
          <p class="language-panel-hint">点击语言可在图表中高亮；较小语言会从“其他”中单独展示。</p>
          <div class="lang-table-scroller">
            <table class="lang-table">
              <thead>
                <tr>
                  <th style="width: 30%">语言</th>
                  <th style="width: 14%">占比</th>
                  <th style="width: 18%">字节数</th>
                  <th style="width: 38%">技术栈版本</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(l, i) in filteredLanguages"
                  :key="l.name"
                  v-motion
                  :class="{ active: selectedChartLanguage === l.name }"
                  :initial="rowEnter"
                  :animate="{ opacity: 1, y: 0 }"
                  :transition="{ type: 'spring', bounce: 0, duration: 0.4, delay: Math.min(0.1 + i * 0.04, 0.5) }"
                >
                  <td>
                    <button
                      class="lang-name-button"
                      type="button"
                      :aria-pressed="selectedChartLanguage === l.name"
                      @click="toggleChartLanguage(l.name)"
                    >
                      <span class="lang-swatch" :style="{ background: l.color }"></span>
                      <span class="lang-name">{{ l.name }}</span>
                    </button>
                  </td>
                  <td class="mono">{{ l.percentage.toFixed(2) }}%</td>
                  <td class="mono muted">{{ formatBytes(l.bytes) }}</td>
                  <td>
                    <div v-if="l.version" class="tech-version-list">
                      <span v-for="item in technologyVersions(l.name, l.version)" :key="item" class="tech-version-chip">
                        <svg
                          v-if="technologyIcon(item)"
                          class="tech-version-icon"
                          viewBox="0 0 24 24"
                          aria-hidden="true"
                          :style="{ color: `#${technologyIcon(item)?.hex}` }"
                        >
                          <path :d="technologyIcon(item)?.path" />
                        </svg>
                        {{ item }}
                      </span>
                    </div>
                    <span v-else class="mono muted">—</span>
                  </td>
                </tr>
                <tr v-if="!filteredLanguages.length">
                  <td colspan="4" class="language-empty">没有匹配的语言</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
      </template>
  </motion.section>

  <footer class="site-footer">
    © {{ copyrightYear }} Linguagram · 作者 Scott · 保留所有权利。
  </footer>

  <ToastHost />
</template>
