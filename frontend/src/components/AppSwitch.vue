<script setup lang="ts">
// Replaces el-switch (live-refresh toggle). The knob slides on a CSS transition
// using Apple's default ease (cubic-bezier(0.22,0.61,0.36,1) ≈ iOS) — this is a
// discrete click→state change, not a gesture, so CSS is fine (§3).
const model = defineModel<boolean>()
withDefaults(defineProps<{ size?: 'small' | 'default' }>(), { size: 'default' })
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="model"
    class="app-switch"
    :class="[size, { on: model }]"
    @click="model = !model"
  >
    <span class="knob" />
  </button>
</template>

<style scoped>
.app-switch {
  position: relative;
  flex: none;
  width: 44px;
  height: 26px;
  border: none;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.16);
  cursor: pointer;
  transition: background 0.2s ease;
}
.app-switch.small { width: 38px; height: 22px; }
.app-switch.on { background: var(--success); }

.knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.25);
  transition: transform 0.28s cubic-bezier(0.22, 0.61, 0.36, 1);
}
.app-switch.small .knob { width: 18px; height: 18px; }
.app-switch.on .knob { transform: translateX(18px); }
.app-switch.small.on .knob { transform: translateX(16px); }

@media (prefers-reduced-motion: reduce) {
  .knob { transition: none; }
}
</style>
