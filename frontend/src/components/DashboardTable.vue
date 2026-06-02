<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

let GetTelemetry: any

const telemetry = ref({
  cpuUsage: 0,
  ramUsage: 0,
  sessions: [] as any[],
  version: '1.0.0'
})

let interval: ReturnType<typeof setInterval>

onMounted(async () => {
  try {
    // Wails JS bindings are injected globally
    GetTelemetry = (window as any).go.main.App.GetTelemetry
    if (GetTelemetry) {
      telemetry.value = await GetTelemetry()
      interval = setInterval(async () => {
        telemetry.value = await GetTelemetry()
      }, 1000)
    }
  } catch(e) {
    console.error("Wails bindings not loaded")
  }
})

onUnmounted(() => {
  if (interval) clearInterval(interval)
})

function getHealthColor(bitrateMbps: number) {
  if (bitrateMbps > 5) return '#10b981' // Green
  if (bitrateMbps < 3) return '#f59e0b' // Yellow
  return '#3b82f6' // Blue (normal)
}

function copyRTSP(streamName: string) {
  navigator.clipboard.writeText(`rtsp://127.0.0.1:8554/${streamName}`)
  alert('Enlace RTSP copiado al portapapeles')
}
</script>

<template>
  <div class="dashboard">
    <div class="metrics-panel">
      <div class="metric">
        <span class="label">Uso de CPU</span>
        <span class="value">{{ telemetry.cpuUsage.toFixed(1) }}%</span>
      </div>
      <div class="metric">
        <span class="label">Uso de RAM</span>
        <span class="value">{{ telemetry.ramUsage }} MB</span>
      </div>
      <div class="metric">
        <span class="label">Red NDI</span>
        <select class="net-select">
          <option>Automático (0.0.0.0)</option>
          <option>en0 (Wi-Fi)</option>
          <option>en1 (Ethernet)</option>
        </select>
      </div>
    </div>
    
    <div class="header">
      <h2>Cámaras LiveCast</h2>
    </div>
    
    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>Origen (RTMP)</th>
            <th>Salud (Bitrate)</th>
            <th>Nombre NDI (Editable)</th>
            <th>Respaldo RTSP</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!telemetry.sessions || telemetry.sessions.length === 0">
            <td colspan="4" class="empty-state">Esperando conexión de cámaras...</td>
          </tr>
          <tr v-for="cam in telemetry.sessions" :key="cam.id">
            <td>live/{{ cam.id }}</td>
            <td>
              <div class="health-indicator">
                <span class="dot" :style="{ backgroundColor: getHealthColor(cam.bitrate) }"></span>
                {{ cam.bitrate.toFixed(2) }} Mbps
              </div>
            </td>
            <td>
              <input type="text" class="ndi-input" v-model="cam.ndiName" />
            </td>
            <td>
              <button class="btn btn-outline" @click="copyRTSP(cam.id)">Copiar Enlace</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.metrics-panel {
  display: flex;
  gap: 20px;
  background-color: var(--panel-bg);
  padding: 15px 20px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
}
.metric {
  display: flex;
  flex-direction: column;
}
.label {
  font-size: 12px;
  color: var(--text-muted);
}
.value {
  font-size: 18px;
  font-weight: bold;
}
.net-select {
  background: var(--bg-color);
  color: white;
  border: 1px solid var(--border-color);
  border-radius: 4px;
  padding: 4px 8px;
}
.table-container {
  background-color: var(--panel-bg);
  border-radius: 8px;
  border: 1px solid var(--border-color);
  padding: 20px;
}
.header { margin-bottom: 5px; }
h2 { margin: 0; font-size: 16px; font-weight: 600; }
.data-table { width: 100%; border-collapse: collapse; }
.data-table th, .data-table td { padding: 12px 16px; text-align: left; border-bottom: 1px solid var(--border-color); }
.data-table th { color: var(--text-muted); font-size: 14px; }
.health-indicator { display: flex; align-items: center; gap: 8px; }
.dot { width: 10px; height: 10px; border-radius: 50%; }
.ndi-input { background: var(--bg-color); color: white; border: 1px solid var(--border-color); padding: 6px; border-radius: 4px; }
.btn { background-color: var(--primary-color); color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; }
.btn-outline { background-color: transparent; border: 1px solid var(--primary-color); color: var(--primary-color); }
.empty-state { text-align: center; color: var(--text-muted); padding: 40px !important; }
</style>
