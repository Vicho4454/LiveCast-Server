<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import LiveVideo from './LiveVideo.vue'
import CameraControls from './CameraControls.vue'

let GetTelemetry: any
let ChangeNDIName: any

const telemetry = ref({
  cpuUsage: 0,
  ramUsage: 0,
  sessions: [] as any[],
  version: '1.0.0',
  ndiEnabled: false
})

const activeControlCam = ref('')

const vmixIp = ref('')
const vmixConnected = ref(false)
const recordingStates = ref<Record<string, boolean>>({})

let interval: ReturnType<typeof setInterval>

onMounted(async () => {
  try {
    // Wails JS bindings are injected globally
    GetTelemetry = (window as any).go?.main?.App?.GetTelemetry
    ChangeNDIName = (window as any).go?.main?.App?.ChangeNDIName
  } catch(e) {}

  const getApiUrl = () => {
    if (window.location.protocol === 'wails:' || window.location.hostname === 'wails.localhost') {
      return 'http://localhost:8080'
    }
    return ''
  }

  const fetchTelemetry = async () => {
    if (GetTelemetry) {
      return await GetTelemetry()
    } else {
      // Fallback para navegador web remoto
      const res = await fetch(getApiUrl() + '/api/status')
      if (!res.ok) throw new Error('Network error')
      return await res.json()
    }
  }

  const update = async () => {
    try {
      telemetry.value = await fetchTelemetry()
      
      const vmixRes = await fetch(getApiUrl() + '/api/tally/vmix/status')
      if (vmixRes.ok) {
        const vmixData = await vmixRes.json()
        vmixConnected.value = vmixData.connected
        if (vmixData.ip && !vmixConnected.value) vmixIp.value = vmixData.ip
      }

      // Sync recording status for each camera
      for (const cam of telemetry.value.sessions) {
        const dvrRes = await fetch(`${getApiUrl()}/api/dvr?id=${cam.id}`)
        if (dvrRes.ok) {
          const dvrData = await dvrRes.json()
          recordingStates.value[cam.id] = dvrData.recording
        }
      }
    } catch(e) {
      console.error("Error fetching telemetry:", e)
    }
  }

  update()
  interval = setInterval(update, 2000)
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

async function updateName(cam: any) {
  if (ChangeNDIName) {
    await ChangeNDIName(cam.id, cam.ndiName)
  }
}

async function connectVmix() {
  if (!vmixIp.value) return
  const getApiUrl = () => {
    return (window.location.protocol === 'wails:' || window.location.hostname === 'wails.localhost') ? 'http://localhost:8080' : ''
  }
  await fetch(getApiUrl() + '/api/tally/vmix', {
    method: 'POST',
    body: JSON.stringify({ ip: vmixIp.value }),
    headers: { 'Content-Type': 'application/json' }
  })
  vmixConnected.value = true
}

async function disconnectVmix() {
  const getApiUrl = () => {
    return (window.location.protocol === 'wails:' || window.location.hostname === 'wails.localhost') ? 'http://localhost:8080' : ''
  }
  await fetch(getApiUrl() + '/api/tally/vmix', { method: 'DELETE' })
  vmixConnected.value = false
}

async function toggleRecording(camId: string) {
  const getApiUrl = () => {
    return (window.location.protocol === 'wails:' || window.location.hostname === 'wails.localhost') ? 'http://localhost:8080' : ''
  }
  
  const isRecording = recordingStates.value[camId]
  const method = isRecording ? 'DELETE' : 'POST'
  
  try {
    const res = await fetch(`${getApiUrl()}/api/dvr?id=${camId}`, { method })
    if (res.ok) {
      recordingStates.value[camId] = !isRecording
    } else {
      alert("Error al cambiar estado de grabación")
    }
  } catch (err) {
    console.error(err)
  }
}
</script>

<template>
  <div class="dashboard">
    <div class="metrics-panel">
      <div class="metric">
        <span class="label">Versión</span>
        <span class="value">{{ telemetry.version }}</span>
      </div>
      <div class="metric">
        <span class="label">Uso de RAM</span>
        <span class="value">{{ telemetry.ramUsage }} MB</span>
      </div>
      <div class="metric">
        <span class="label">Estado NDI</span>
        <span class="value" :style="{ color: telemetry.ndiEnabled ? '#10b981' : '#ef4444' }">
          {{ telemetry.ndiEnabled ? 'ACTIVO' : 'INACTIVO' }}
        </span>
      </div>
    </div>
    
    <div class="metrics-panel vmix-panel">
      <div class="metric">
        <span class="label">Integración vMix (Tally Lights)</span>
        <div class="vmix-controls">
          <input type="text" v-model="vmixIp" placeholder="Ej: 192.168.1.50" class="ndi-input" :disabled="vmixConnected" />
          <button v-if="!vmixConnected" class="btn" style="background-color: #10b981;" @click="connectVmix">Conectar a vMix</button>
          <button v-else class="btn" style="background-color: #ef4444;" @click="disconnectVmix">Desconectar</button>
          <span v-if="vmixConnected" class="status-badge connected">● Tally Activo</span>
        </div>
      </div>
    </div>
    
    <div class="header">
      <h2>Cámaras LiveCast ({{ telemetry.sessions?.length || 0 }})</h2>
    </div>
    
    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>Vista Previa</th>
            <th>Origen (RTMP)</th>
            <th>Salud (Bitrate)</th>
            <th>Batería</th>
            <th>Nombre NDI (Editable)</th>
            <th>Grabación (DVR)</th>
            <th>Respaldo RTSP</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!telemetry.sessions || telemetry.sessions.length === 0">
            <td colspan="7" class="empty-state">Esperando conexión de cámaras...</td>
          </tr>
          <tr v-for="cam in telemetry.sessions" :key="cam.id">
            <td class="preview-cell">
              <LiveVideo :streamId="cam.id" class="mini-preview" />
            </td>
            <td>{{ cam.id }}</td>
            <td>
              <div class="health-indicator">
                <span class="dot" :style="{ backgroundColor: getHealthColor(cam.bitrate) }"></span>
                {{ cam.bitrate.toFixed(2) }} Mbps
              </div>
            </td>
            <td>
              <div v-if="cam.hasTelemetry" class="battery-indicator">
                <span :style="{ color: cam.batteryLevel <= 20 ? '#ef4444' : '#10b981' }">
                  {{ cam.batteryLevel }}%
                </span>
                <span v-if="cam.isCharging" title="Cargando"> ⚡</span>
              </div>
              <div v-else class="text-muted">N/A</div>
            </td>
            <td>
              <input type="text" v-model="cam.ndiName" @change="updateName(cam)" class="ndi-input" placeholder="Ej: Camara Principal" />
            </td>
            <td>
              <button 
                class="btn-outline" 
                :class="{ 'recording-active': recordingStates[cam.id] }"
                @click="toggleRecording(cam.id)"
              >
                {{ recordingStates[cam.id] ? '⏹️ Detener Grabación' : '⏺️ Grabar Local' }}
              </button>
            </td>
            <td>
              <button class="btn-outline" @click="copyRTSP(cam.id)">Copiar RTSP</button>
              <button class="btn-outline settings-btn" @click="activeControlCam = cam.id" style="margin-left: 8px;">🎛️ Ajustes</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Modal de Controles de Cámara -->
    <CameraControls v-if="activeControlCam" :streamId="activeControlCam" :onClose="() => activeControlCam = ''" />
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
.recording-active {
  background-color: #ef4444 !important;
  color: white !important;
  border-color: #ef4444 !important;
  animation: pulse-red 2s infinite;
}

@keyframes pulse-red {
  0% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.7); }
  70% { box-shadow: 0 0 0 6px rgba(239, 68, 68, 0); }
  100% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0); }
}

@media (max-width: 1024px) { }
.dot { width: 10px; height: 10px; border-radius: 50%; }
.ndi-input { background: var(--bg-color); color: white; border: 1px solid var(--border-color); padding: 6px; border-radius: 4px; }
.btn { background-color: var(--primary-color); color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; }
.btn-outline { background-color: transparent; border: 1px solid var(--primary-color); color: var(--primary-color); }
.empty-state { text-align: center; color: var(--text-muted); padding: 40px !important; }
.preview-cell { width: 100px; }
.mini-preview { width: 90px; height: 50px; object-fit: contain; border-radius: 4px; background: #000; border: 1px solid #333; }
.vmix-panel { background-color: rgba(16, 185, 129, 0.05); border-color: rgba(16, 185, 129, 0.2); }
.vmix-controls { display: flex; gap: 10px; margin-top: 5px; align-items: center; }
.status-badge { font-size: 12px; font-weight: bold; }
.status-badge.connected { color: #10b981; }
</style>
