<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'

const props = defineProps<{ streamId: string }>()
const imgSrc = ref('')
let active = true

const getBaseUrl = () => (window as any).go ? 'http://localhost:3000' : ''

const fetchFrame = async () => {
  if (!active) return
  try {
    const res = await fetch(`${getBaseUrl()}/api/frame?id=${props.streamId}`)
    if (res.ok) {
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      if (imgSrc.value) URL.revokeObjectURL(imgSrc.value)
      imgSrc.value = url
    }
  } catch (e) {
    // silently fail
  }
  if (active) {
    setTimeout(fetchFrame, 1000 / 15) // ~15 fps
  }
}

onMounted(() => {
  fetchFrame()
})

onUnmounted(() => {
  active = false
  if (imgSrc.value) URL.revokeObjectURL(imgSrc.value)
})
</script>

<template>
  <img :src="imgSrc || getBaseUrl() + '/api/frame?id='+streamId" alt="Preview" />
</template>
