<script setup lang="ts">
import { ref, onMounted } from 'vue'
import DashboardTable from './components/DashboardTable.vue'
import StudioView from './components/StudioView.vue'
import { EventsOn } from '../wailsjs/runtime/runtime'

const logs = ref<string[]>([])
const activeTab = ref('dashboard')

onMounted(() => {
  EventsOn('backend-log', (logLine: string) => {
    logs.value.push(logLine)
    if (logs.value.length > 50) {
      logs.value.shift()
    }
    // auto-scroll
    setTimeout(() => {
      const el = document.getElementById('log-container')
      if (el) el.scrollTop = el.scrollHeight
    }, 50)
  })
})
</script>

<template>
  <div class="app-container">
    <header class="topbar">
      <div class="logo">LC</div>
      <div class="title">LiveCast Server</div>
      <div class="tabs">
        <button :class="{ active: activeTab === 'dashboard' }" @click="activeTab = 'dashboard'">Panel de Control</button>
        <button :class="{ active: activeTab === 'studio' }" @click="activeTab = 'studio'">Modo Estudio (Multicam)</button>
      </div>
    </header>
    <main class="content">
      <DashboardTable v-show="activeTab === 'dashboard'" />
      <StudioView v-show="activeTab === 'studio'" />
    </main>
    <footer class="log-footer">
      <div id="log-container" class="log-container">
        <div v-for="(line, i) in logs" :key="i" class="log-line">{{ line }}</div>
        <div v-if="logs.length === 0" class="log-line" style="color: #666;">Esperando logs de MediaMTX...</div>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.app-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
}
.tabs {
  margin-left: auto;
  display: flex;
  gap: 10px;
}
.tabs button {
  background: transparent;
  color: #ccc;
  border: 1px solid transparent;
  padding: 6px 12px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}
.tabs button:hover {
  background: rgba(255,255,255,0.1);
}
.tabs button.active {
  background: var(--primary-color);
  color: white;
  border-color: var(--primary-color);
}
.content {
  flex: 1;
  overflow: auto;
}
.log-footer {
  height: 150px;
  background: #1e1e1e;
  border-top: 1px solid #333;
  padding: 8px;
}
.log-container {
  height: 100%;
  overflow-y: auto;
  font-family: monospace;
  font-size: 12px;
  color: #0f0;
}
.log-line {
  margin: 2px 0;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
