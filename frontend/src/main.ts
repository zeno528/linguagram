import { createApp } from 'vue'
import { MotionPlugin } from 'motion-v'
import App from './App.vue'
import './style.css'

// MotionPlugin registers the v-motion directive globally so any element can
// animate declaratively; <AnimatePresence> (imported per component) pairs with
// it to drive exit animations for v-if / v-for removals.
createApp(App).use(MotionPlugin).mount('#app')