import { computed } from 'vue'
import { useReducedMotion } from 'motion-v'

// Keep only rules shared by more than one component here. Element-specific
// geometry belongs beside the element that uses it.
export const standardSpring = { type: 'spring' as const, bounce: 0, duration: 0.4 }
export const quickSpring = { type: 'spring' as const, bounce: 0, duration: 0.36 }

export function useMotionPresets() {
  const reduce = useReducedMotion()
  const expandInitial = computed(() =>
    reduce.value ? { opacity: 0 } : { height: 0, opacity: 0, y: -8 },
  )
  const expandAnimate = { height: 'auto', opacity: 1, y: 0 }
  function expandState(isOpen: boolean) {
    if (reduce.value) return { opacity: isOpen ? 1 : 0 }
    return isOpen ? expandAnimate : { height: 0, opacity: 0, y: -8 }
  }

  return {
    reduce,
    standardSpring,
    quickSpring,
    expandInitial,
    expandAnimate,
    expandState,
  }
}
