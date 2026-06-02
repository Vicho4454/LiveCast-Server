<script setup lang="ts">
import DashboardTable from './components/DashboardTable.vue'
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
  </div>
</template>
