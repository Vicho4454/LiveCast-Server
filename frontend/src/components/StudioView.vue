<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import LiveVideo from './LiveVideo.vue'

let GetTelemetry: any

const sessions = ref<any[]>([])
let interval: ReturnType<typeof setInterval>

onMounted(async () => {
  try {
    GetTelemetry = (window as any).go?.main?.App?.GetTelemetry
  } catch(e) {}

  const fetchTelemetry = async () => {
    if (GetTelemetry) {
      return await GetTelemetry()
    } else {
      const res = await fetch('/api/status')
      if (!res.ok) throw new Error('Network error')
      return await res.json()
    }
  }

  const update = async () => {
    try {
      const data = await fetchTelemetry()
      sessions.value = data.sessions || []
    } catch(e) {
      console.error(e)
    }
  }

  update()
  interval = setInterval(update, 2000)
})

onUnmounted(() => {
  if (interval) clearInterval(interval)
})
</script>

<template>
  <div class="studio-container">
    <div v-if="sessions.length === 0" class="empty-state">
      <h3>No hay cámaras conectadas</h3>
      <p>Conecta una cámara para iniciar el Modo Estudio</p>
    </div>
    <div v-else class="grid">
      <div v-for="cam in sessions" :key="cam.id" class="camera-feed">
        <div class="feed-header">
          <span class="cam-name">{{ cam.ndiName || cam.id }}</span>
          <span class="status-dot"></span>
        </div>
        <LiveVideo :streamId="cam.id" class="video-preview" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.studio-container {
  padding: 20px;
  height: 100%;
  display: flex;
  flex-direction: column;
}
.empty-state {
  margin: auto;
  text-align: center;
  color: var(--text-muted);
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  gap: 20px;
  width: 100%;
  height: 100%;
}
.camera-feed {
  background: #000;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  position: relative;
}
.feed-header {
  position: absolute;
  top: 10px;
  left: 10px;
  background: rgba(0,0,0,0.7);
  padding: 4px 12px;
  border-radius: 20px;
  display: flex;
  align-items: center;
  gap: 8px;
  z-index: 10;
}
.cam-name {
  color: white;
  font-weight: 600;
  font-size: 14px;
}
.status-dot {
  width: 8px;
  height: 8px;
  background-color: #ef4444; /* Red rec dot */
  border-radius: 50%;
  animation: pulse 1.5s infinite;
}
.video-preview {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #111;
}

@keyframes pulse {
  0% { opacity: 1; }
  50% { opacity: 0.3; }
  100% { opacity: 1; }
}
</style>
