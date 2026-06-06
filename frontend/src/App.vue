<script setup lang="ts">
import { ref, onMounted } from 'vue'
import DashboardTable from './components/DashboardTable.vue'
import { EventsOn } from '../wailsjs/runtime/runtime'

const logs = ref<string[]>([])

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
    </header>
    <main class="content">
      <DashboardTable />
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
