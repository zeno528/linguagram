import { ref } from 'vue'

export type ToastKind = 'success' | 'warning' | 'error' | 'info'

export interface ToastItem {
  id: number
  kind: ToastKind
  message: string
}

// Module-scoped singleton queue: one ToastHost renders for the whole app, and
// any component can push. This is the drop-in replacement for Element Plus's
// global ElMessage API (called imperatively from event handlers / catch blocks).
const toasts = ref<ToastItem[]>([])
let nextId = 1

function push(kind: ToastKind, message: string, duration = 2600): number {
  const id = nextId++
  toasts.value = [...toasts.value, { id, kind, message }]
  // Auto-dismiss after the dwell time. The leave animation itself is owned by
  // ToastHost's <AnimatePresence>, so we only remove from the queue here.
  setTimeout(() => dismiss(id), duration)
  return id
}

function dismiss(id: number): void {
  toasts.value = toasts.value.filter((t) => t.id !== id)
}

export function useToast() {
  return {
    toasts,
    dismiss,
    success: (m: string) => push('success', m),
    warning: (m: string) => push('warning', m),
    // Errors linger a little longer so the user can finish reading them.
    error: (m: string, d = 3800) => push('error', m, d),
    info: (m: string) => push('info', m),
  }
}
