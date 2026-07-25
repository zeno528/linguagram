<script setup lang="ts">
import { computed } from 'vue'
import { AnimatePresence, useReducedMotion } from 'motion-v'
import { useToast } from '../composables/useToast'

// Replaces ElMessage. A single host is mounted once in App.vue and teleports
// to <body>; any component pushes via useToast(). Enter/exit run on a spring
// (§4 critically-damped default), downgraded to an opacity cross-fade when the
// user prefers reduced motion (§14).
const { toasts, dismiss } = useToast()
const reduce = useReducedMotion()

const enter = computed(() => (reduce.value ? { opacity: 0 } : { opacity: 0, y: -16, scale: 0.96 }))
const exit = computed(() => (reduce.value ? { opacity: 0 } : { opacity: 0, y: -12, scale: 0.98 }))
const spring = { type: 'spring', bounce: 0, duration: 0.4 }
</script>

<template>
  <Teleport to="body">
    <div class="toast-stack" aria-live="polite" role="status">
      <AnimatePresence>
        <div
          v-for="t in toasts"
          :key="t.id"
          v-motion
          :initial="enter"
          :animate="{ opacity: 1, y: 0, scale: 1 }"
          :exit="exit"
          :transition="spring"
          class="toast"
          :class="t.kind"
          @click="dismiss(t.id)"
        >
          <span class="toast-dot" aria-hidden="true" />
          <span class="toast-msg">{{ t.message }}</span>
        </div>
      </AnimatePresence>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-stack {
  position: fixed;
  top: 24px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: center;
  pointer-events: none;
}
.toast {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: 10px;
  max-width: min(90vw, 460px);
  padding: 12px 18px;
  border-radius: 14px;
  font-size: 14px;
  font-weight: 500;
  color: var(--fg);
  background: var(--surface);
  -webkit-backdrop-filter: var(--blur);
  backdrop-filter: var(--blur);
  border-top: 1px solid var(--highlight-edge);
  box-shadow: var(--keyline), var(--shadow-1);
  cursor: pointer;
}
.toast-dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
}
.toast.success .toast-dot { background: var(--success); }
.toast.warning .toast-dot { background: #ff9f0a; } /* systemOrange */
.toast.error   .toast-dot { background: var(--danger); }

@media (prefers-reduced-transparency: reduce) {
  .toast {
    background: var(--surface-solid);
    -webkit-backdrop-filter: none;
    backdrop-filter: none;
  }
}
</style>
