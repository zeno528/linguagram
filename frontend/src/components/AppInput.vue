<script setup lang="ts">
// Replaces el-input (path field). Focus feedback is a color/shadow change — a
// state transition, not a gesture — so a CSS transition is appropriate here
// (§3 only forbids CSS transitions for gesture-driven, interruptible motion).
const model = defineModel<string>()
withDefaults(defineProps<{ placeholder?: string; clearable?: boolean }>(), {
  placeholder: '',
  clearable: true,
})
defineEmits<{ (e: 'enter'): void }>()
</script>

<template>
  <div class="app-input">
    <input
      v-model="model"
      type="text"
      spellcheck="false"
      autocomplete="off"
      :placeholder="placeholder"
      class="ai-field"
      @keyup.enter="$emit('enter')"
    />
    <button
      v-if="clearable && model"
      class="ai-clear"
      type="button"
      aria-label="清除"
      @click="model = ''"
    >×</button>
  </div>
</template>

<style scoped>
.app-input {
  display: flex;
  align-items: center;
  height: 40px;
  background: var(--surface-inset);
  border-radius: 12px;
  box-shadow: inset 0 0 0 1px var(--hairline);
  transition: box-shadow 0.2s ease, background 0.2s ease;
}
.app-input:focus-within {
  background: var(--surface-solid);
  box-shadow: inset 0 0 0 1.5px var(--accent), 0 0 0 4px var(--accent-soft);
}
.ai-field {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 0 14px;
  border: none;
  background: transparent;
  font-family: inherit;
  font-size: 15px;
  color: var(--fg);
  outline: none;
}
.ai-field::placeholder { color: var(--muted); }
.ai-clear {
  flex: none;
  margin-right: 8px;
  width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.18);
  color: #fff;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  transition: transform 0.12s ease, background 0.12s ease;
}
.ai-clear:hover { background: rgba(0, 0, 0, 0.3); }
.ai-clear:active { transform: scale(0.85); }
</style>
