<script setup>
// Main layout container
</script>

<template>
  <div class="app-shell">
    <router-view v-slot="{ Component }">
      <transition name="fade" mode="out-in">
        <component :is="Component" />
      </transition>
    </router-view>
  </div>
</template>

<style>
/* Global layout styles */
.app-shell {
  min-height: 100vh;
}
</style>
