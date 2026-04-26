<script setup>
import { computed } from 'vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps({
  job: {
    type: Object,
    required: true
  }
})

const statusMessage = computed(() => {
  switch (props.job.status) {
    case 'pending': return 'Initializing upload...'
    case 'uploading': return `Uploading parts... ${props.job.progress}%`
    case 'queued': return 'In queue for processing...'
    case 'processing': return 'Transcoding video...'
    case 'downloading': return 'Preparing files...'
    case 'completed': return 'Success! Video is ready.'
    case 'failed': return `Error: ${props.job.error || 'Processing failed'}`
    case 'cancelled': return 'Job was cancelled by user'
    default: return 'Processing...'
  }
})

const isError = computed(() => props.job.status === 'failed')
</script>

<template>
  <div class="progress-panel" :class="{ 'is-error': isError }">
    <div class="header">
      <div class="job-meta">
        <h3>{{ job.name }}</h3>
        <span class="job-id" v-if="job.id">ID: {{ job.id }}</span>
      </div>
      <StatusBadge :status="job.status" />
    </div>

    <div class="progress-container">
      <div class="progress-bar-bg">
        <div 
          class="progress-bar-fill" 
          :class="job.status"
          :style="{ width: `${job.progress}%` }"
        ></div>
      </div>
      <div class="status-text">
        <span class="message">{{ statusMessage }}</span>
        <span class="percentage" v-if="job.status === 'uploading'">{{ job.progress }}%</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.progress-panel {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius);
  padding: 1.5rem;
  box-shadow: var(--shadow-sm);
}

.progress-panel.is-error {
  border-color: rgba(239, 68, 68, 0.2);
  background: rgba(239, 68, 68, 0.01);
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
}

.job-meta h3 {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.job-id {
  font-size: 12px;
  color: var(--text-tertiary);
  font-family: monospace;
}

.progress-container {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.progress-bar-bg {
  height: 8px;
  background: var(--bg-color);
  border-radius: 4px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  background: var(--accent-color);
  transition: width 300ms ease-out;
}

.progress-bar-fill.uploading { background: var(--accent-color); }
.progress-bar-fill.processing { background: var(--status-processing); animation: pulse 2s infinite; }
.progress-bar-fill.completed { background: var(--status-completed); width: 100% !important; }
.progress-bar-fill.failed { background: var(--status-failed); }

@keyframes pulse {
  0% { opacity: 1; }
  50% { opacity: 0.6; }
  100% { opacity: 1; }
}

.status-text {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 500;
}

.message {
  color: var(--text-secondary);
}

.percentage {
  color: var(--accent-color);
}
</style>
