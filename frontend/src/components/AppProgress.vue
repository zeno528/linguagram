<script setup lang="ts">
// Replaces el-progress. Two modes mirror the original: indeterminate (path
// scan — unknown duration) shows a travelling stripe; determinate (drag-drop
// file read) shows a fill width driven by real progress. Width changes use a
// CSS transition — it's a value update, not an interruptible gesture (§3).
withDefaults(
  defineProps<{
    percentage?: number
    indeterminate?: boolean
    status?: 'success' | undefined
    showText?: boolean
  }>(),
  { percentage: 0, indeterminate: false, status: undefined, showText: false },
)
</script>

<template>
  <div class="app-progress">
    <div v-if="showText && !indeterminate" class="ap-text">
      {{ percentage }}<span class="pct">%</span>
    </div>
    <div class="track">
      <div
        v-if="!indeterminate"
        class="fill"
        :class="status"
        :style="{ width: Math.max(0, Math.min(100, percentage)) + '%' }"
      />
      <div v-else class="fill indeterminate" />
    </div>
  </div>
</template>

<style scoped>
.app-progress { width: 100%; }
.ap-text {
  font-size: 13px;
  font-weight: 600;
  color: var(--muted);
  font-variant-numeric: tabular-nums;
  margin-bottom: 6px;
  letter-spacing: -0.01em;
}
.ap-text .pct { color: var(--muted); margin-left: 1px; }

.track {
  position: relative;
  height: 8px;
  border-radius: 999px;
  background: var(--surface-inset);
  box-shadow: inset 0 0 0 1px var(--hairline);
  overflow: hidden;
}
.fill {
  height: 100%;
  border-radius: 999px;
  background: var(--accent);
  transition: width 0.5s cubic-bezier(0.22, 0.61, 0.36, 1);
}
.fill.success { background: var(--success); }
.fill.indeterminate {
  width: 35%;
  background: var(--accent);
  animation: travel 1.1s cubic-bezier(0.65, 0, 0.35, 1) infinite;
}
@keyframes travel {
  0%   { transform: translateX(-120%); }
  100% { transform: translateX(320%); }
}

@media (prefers-reduced-motion: reduce) {
  .fill { transition: none; }
  .fill.indeterminate { animation: none; width: 100%; opacity: 0.55; }
}
</style>
