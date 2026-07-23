<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{
  streamId: string
  onClose: () => void
}>()

const zoom = ref(1.0)
const focus = ref(0.5)
const autoFocus = ref(true)
const exposure = ref(0)
const iso = ref(400)
const autoIso = ref(true)
const shutter = ref(60) // represents 1/60
const autoShutter = ref(true)
const wb = ref(5200) // Kelvin
const autoWb = ref(true)
const sending = ref(false)
const errorMsg = ref('')

const getBaseUrl = () => (window as any).go ? 'http://localhost:3000' : ''

const sendCommand = async (action: string, value: any) => {
  sending.value = true
  errorMsg.value = ''
  try {
    const res = await fetch(`${getBaseUrl()}/api/camera/control?id=${props.streamId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, value })
    })
    if (!res.ok) {
      errorMsg.value = 'Teléfono no conectado al panel de control'
    }
  } catch (e) {
    errorMsg.value = 'Error de red al enviar comando'
  }
  sending.value = false
}

// Watchers for sliders (using a small debounce could be good, but native range sliders only fire on input)
let zoomTimeout: any
const updateZoom = () => {
  clearTimeout(zoomTimeout)
  zoomTimeout = setTimeout(() => sendCommand('zoom', parseFloat(zoom.value.toString())), 100)
}

let focusTimeout: any
const updateFocus = () => {
  if (autoFocus.value) return
  clearTimeout(focusTimeout)
  focusTimeout = setTimeout(() => sendCommand('focus', parseFloat(focus.value.toString())), 100)
}

const toggleAutoFocus = () => {
  autoFocus.value = !autoFocus.value
  sendCommand('autoFocus', autoFocus.value)
}

let exposureTimeout: any
const updateExposure = () => {
  clearTimeout(exposureTimeout)
  exposureTimeout = setTimeout(() => sendCommand('exposure', parseInt(exposure.value.toString())), 100)
}

let isoTimeout: any
const updateIso = () => {
  if (autoIso.value) return
  clearTimeout(isoTimeout)
  isoTimeout = setTimeout(() => sendCommand('iso', parseInt(iso.value.toString())), 100)
}

const toggleAutoIso = () => {
  autoIso.value = !autoIso.value
  sendCommand('autoIso', autoIso.value)
}

let shutterTimeout: any
const updateShutter = () => {
  if (autoShutter.value) return
  clearTimeout(shutterTimeout)
  shutterTimeout = setTimeout(() => sendCommand('shutter', parseInt(shutter.value.toString())), 100)
}

const toggleAutoShutter = () => {
  autoShutter.value = !autoShutter.value
  sendCommand('autoShutter', autoShutter.value)
}

let wbTimeout: any
const updateWb = () => {
  if (autoWb.value) return
  clearTimeout(wbTimeout)
  wbTimeout = setTimeout(() => sendCommand('wb', parseInt(wb.value.toString())), 100)
}

const toggleAutoWb = () => {
  autoWb.value = !autoWb.value
  sendCommand('autoWb', autoWb.value)
}
</script>

<template>
  <div class="modal-overlay" @click.self="onClose">
    <div class="modal-content">
      <div class="modal-header">
        <h3>🎛️ Control Remoto: {{ streamId }}</h3>
        <button class="close-btn" @click="onClose">✕</button>
      </div>

      <div v-if="errorMsg" class="error-banner">
        {{ errorMsg }}
      </div>

      <div class="control-group">
        <label>🔍 Zoom ({{ zoom }}x)</label>
        <input type="range" min="1.0" max="10.0" step="0.1" v-model="zoom" @input="updateZoom" class="slider" />
      </div>

      <div class="control-group">
        <div class="flex-between">
          <label>🎯 Enfoque ({{ focus }})</label>
          <button :class="['toggle-btn', { active: autoFocus }]" @click="toggleAutoFocus">
            {{ autoFocus ? 'Automático' : 'Manual' }}
          </button>
        </div>
        <input type="range" min="0" max="10" step="0.1" v-model.number="focus" @input="updateFocus" class="slider" :disabled="autoFocus" />
      </div>

      <div class="control-group">
        <label>☀️ Exposición EV ({{ exposure > 0 ? '+' : '' }}{{ exposure }})</label>
        <input type="range" min="-8" max="8" step="1" v-model="exposure" @input="updateExposure" class="slider" />
      </div>

      <div class="control-group">
        <div class="flex-between">
          <label>📸 ISO ({{ iso }})</label>
          <button :class="['toggle-btn', { active: autoIso }]" @click="toggleAutoIso">
            {{ autoIso ? 'Automático' : 'Manual' }}
          </button>
        </div>
        <input type="range" min="50" max="3200" step="50" v-model.number="iso" @input="updateIso" class="slider" :disabled="autoIso" />
      </div>

      <div class="control-group">
        <div class="flex-between">
          <label>⏱️ Obturador (1/{{ shutter }}s)</label>
          <button :class="['toggle-btn', { active: autoShutter }]" @click="toggleAutoShutter">
            {{ autoShutter ? 'Automático' : 'Manual' }}
          </button>
        </div>
        <input type="range" min="1" max="1000" step="1" v-model.number="shutter" @input="updateShutter" class="slider" :disabled="autoShutter" />
      </div>

      <div class="control-group">
        <div class="flex-between">
          <label>🌡️ Balance de Blancos ({{ wb }}K)</label>
          <button :class="['toggle-btn', { active: autoWb }]" @click="toggleAutoWb">
            {{ autoWb ? 'Automático' : 'Manual' }}
          </button>
        </div>
        <input type="range" min="2500" max="9000" step="100" v-model.number="wb" @input="updateWb" class="slider" :disabled="autoWb" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}
.modal-content {
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  width: 450px;
  max-width: 90vw;
  max-height: 90vh;
  overflow-y: auto;
  padding: 24px;
  box-shadow: 0 10px 30px rgba(0,0,0,0.5);
}
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}
.modal-header h3 {
  margin: 0;
  font-weight: 600;
  color: white;
}
.close-btn {
  background: transparent;
  border: none;
  color: #888;
  font-size: 20px;
  cursor: pointer;
}
.close-btn:hover {
  color: white;
}
.error-banner {
  background: rgba(239, 68, 68, 0.2);
  color: #ef4444;
  padding: 10px;
  border-radius: 6px;
  margin-bottom: 20px;
  font-size: 13px;
  border: 1px solid rgba(239, 68, 68, 0.4);
}
.control-group {
  margin-bottom: 24px;
}
.control-group label {
  display: block;
  margin-bottom: 8px;
  font-weight: 500;
  color: #ccc;
  font-size: 14px;
}
.flex-between {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.flex-between label {
  margin-bottom: 0;
}
.slider {
  -webkit-appearance: none;
  width: 100%;
  height: 6px;
  background: #333;
  border-radius: 4px;
  outline: none;
}
.slider:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.slider::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--primary-color);
  cursor: pointer;
  transition: transform 0.1s;
}
.slider::-webkit-slider-thumb:hover {
  transform: scale(1.2);
}
.slider:disabled::-webkit-slider-thumb {
  background: #666;
}
.toggle-btn {
  background: #222;
  border: 1px solid #444;
  color: #aaa;
  padding: 4px 10px;
  border-radius: 12px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
}
.toggle-btn.active {
  background: var(--primary-color);
  border-color: var(--primary-color);
  color: white;
}
</style>
