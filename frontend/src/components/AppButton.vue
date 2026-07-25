<script setup lang="ts">
// Replaces el-button. The Apple-correct feel comes from one rule (§1): feedback
// lives on the press and is instant — :active scales the button immediately,
// with NO transition on the transform (a transition here would add latency to
// the most important feedback in the UI).
withDefaults(
  defineProps<{
    variant?: 'primary' | 'plain'
    loading?: boolean
    disabled?: boolean
  }>(),
  { variant: 'primary', loading: false, disabled: false },
)
</script>

<template>
  <button
    class="app-btn"
    :class="variant"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
  >
    <span v-if="loading" class="spinner" aria-hidden="true" />
    <slot />
  </button>
</template>

<style scoped>
.app-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 40px;
  padding: 0 22px;
  border: none;
  border-radius: 12px;
  font-family: inherit;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: -0.01em;
  cursor: pointer;
  white-space: nowrap;
  /* NOTE: no transition on transform — :active must be instantaneous (§1). */
}
.app-btn:active { transform: scale(0.96); }
.app-btn:disabled { opacity: 0.5; cursor: default; }
.app-btn:active:disabled { transform: none; }

.app-btn.primary {
  background: var(--accent);
  color: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08), 0 4px 14px rgba(0, 113, 227, 0.28);
}
.app-btn.primary:active { background: var(--accent-press); }

.app-btn.plain {
  background: var(--surface-inset);
  color: var(--fg);
  box-shadow: inset 0 0 0 1px var(--hairline-strong);
}

.spinner {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid rgba(255, 255, 255, 0.4);
  border-top-color: #fff;
  animation: spin 0.7s linear infinite;
}
.app-btn.plain .spinner {
  border-color: rgba(0, 0, 0, 0.18);
  border-top-color: var(--fg);
}
@keyframes spin { to { transform: rotate(360deg); } }

@media (prefers-reduced-motion: reduce) {
  .app-btn:active { transform: none; }
  .spinner { animation-duration: 1.4s; }
}
</style>
