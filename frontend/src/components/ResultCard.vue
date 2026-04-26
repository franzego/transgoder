<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../services/api'

const props = defineProps({
  jobId: {
    type: String,
    required: true
  }
})

const urls = ref({
  source: null,
  output: null,
  download: api.getDownloadUrl(props.jobId)
})

const loading = ref(true)

onMounted(async () => {
  try {
    const [sourceRes, outputRes] = await Promise.all([
      api.getSourceUrl(props.jobId),
      api.getOutputUrl(props.jobId)
    ])
    urls.value.source = sourceRes.metadata.source_url
    urls.value.output = outputRes.metadata.output_url
  } catch (err) {
    console.error('Failed to fetch URLs:', err)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="result-card">
    <div class="result-header">
      <div class="success-icon">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
      <h4>Transcoding Complete</h4>
    </div>
    
    <div v-if="loading" class="loading-state">
      Generating access links...
    </div>

    <div v-else class="actions-grid">
      <a :href="urls.source" target="_blank" class="action-item">
        <div class="action-icon">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>
            <circle cx="12" cy="12" r="3"></circle>
          </svg>
        </div>
        <span>View Source</span>
      </a>

      <a :href="urls.output" target="_blank" class="action-item">
        <div class="action-icon accent">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18"></rect>
            <line x1="7" y1="2" x2="7" y2="22"></line>
            <line x1="17" y1="2" x2="17" y2="22"></line>
            <line x1="2" y1="12" x2="22" y2="12"></line>
            <line x1="2" y1="7" x2="7" y2="7"></line>
            <line x1="2" y1="17" x2="7" y2="17"></line>
            <line x1="17" y1="17" x2="22" y2="17"></line>
            <line x1="17" y1="7" x2="22" y2="7"></line>
          </svg>
        </div>
        <span>View Result</span>
      </a>

      <a :href="urls.download" download class="action-item">
        <div class="action-icon success">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
            <polyline points="7 10 12 15 17 10"></polyline>
            <line x1="12" y1="15" x2="12" y2="3"></line>
          </svg>
        </div>
        <span>Download</span>
      </a>
    </div>
  </div>
</template>

<style scoped>
.result-card {
  background: #f8fafc;
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius);
  padding: 1.5rem;
}

.result-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1.5rem;
}

.success-icon {
  width: 32px;
  height: 32px;
  background: var(--status-completed);
  color: white;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.result-header h4 {
  font-size: 15px;
  font-weight: 600;
}

.loading-state {
  font-size: 13px;
  color: var(--text-secondary);
  font-style: italic;
}

.actions-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
}

.action-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem;
  background: white;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  transition: all var(--transition-speed);
  text-decoration: none;
  color: var(--text-primary);
}

.action-item:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
  border-color: var(--text-tertiary);
}

.action-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-color);
  border-radius: 8px;
  color: var(--text-secondary);
}

.action-icon.accent { color: var(--accent-color); background: var(--accent-soft); }
.action-icon.success { color: var(--status-completed); background: rgba(16, 185, 129, 0.05); }

.action-item span {
  font-size: 12px;
  font-weight: 500;
}

@media (max-width: 500px) {
  .actions-grid {
    grid-template-columns: 1fr;
  }
}
</style>
