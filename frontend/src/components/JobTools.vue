<script setup>
import { ref } from 'vue'
import { api } from '../services/api'
import StatusBadge from './StatusBadge.vue'

const lookupId = ref('')
const result = ref(null) // { status, sourceUrl, outputUrl, error }
const loading = ref(false)

const getStatus = async () => {
  if (!lookupId.value) return
  loading.value = true
  result.value = null
  
  try {
    const res = await api.getStatus(lookupId.value)
    result.value = {
      id: lookupId.value,
      status: res.metadata.status,
      error: null
    }
  } catch (err) {
    result.value = { error: err.message }
  } finally {
    loading.value = false
  }
}

const fetchUrl = async (type) => {
  if (!lookupId.value) return
  loading.value = true
  
  try {
    let url = ''
    if (type === 'source') {
      const res = await api.getSourceUrl(lookupId.value)
      url = res.metadata.source_url
    } else {
      const res = await api.getOutputUrl(lookupId.value)
      url = res.metadata.output_url
    }
    window.open(url, '_blank')
  } catch (err) {
    result.value = { ...result.value, error: err.message }
  } finally {
    loading.value = false
  }
}

const downloadStream = () => {
  if (!lookupId.value) return
  window.location.href = api.getDownloadUrl(lookupId.value)
}
</script>

<template>
  <div class="job-tools">
    <div class="tool-section">
      <h3>Job Tools</h3>
      <p>Manage existing transcoding jobs</p>
      
      <div class="input-wrapper">
        <input 
          type="text" 
          v-model="lookupId" 
          placeholder="Enter Job ID..." 
          @keyup.enter="getStatus"
        />
        <button class="btn-search" @click="getStatus" :disabled="loading">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8"></circle>
            <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
          </svg>
        </button>
      </div>
    </div>

    <div v-if="result" class="tool-result">
      <div v-if="result.error" class="error-msg">
        {{ result.error }}
      </div>
      
      <div v-else class="status-block">
        <div class="status-row">
          <span>Current Status</span>
          <StatusBadge :status="result.status" />
        </div>

        <div class="action-buttons">
          <button class="btn-tool" @click="fetchUrl('source')">Source URL</button>
          <button class="btn-tool" @click="fetchUrl('output')">Output URL</button>
          <button class="btn-tool btn-download" @click="downloadStream">Download Stream</button>
        </div>
      </div>
    </div>
    
    <div v-else-if="!loading" class="empty-state">
      Enter a Job ID to retrieve metadata and files.
    </div>
    
    <div v-if="loading" class="loader">
      Working...
    </div>
  </div>
</template>

<style scoped>
.job-tools {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius);
  padding: 1.5rem;
  box-shadow: var(--shadow-sm);
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

h3 {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 0.25rem;
}

p {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 1rem;
}

.input-wrapper {
  display: flex;
  gap: 0.5rem;
}

input {
  flex: 1;
  padding: 0.6rem 0.75rem;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  font-size: 13px;
  background: var(--bg-color);
}

.btn-search {
  background: var(--accent-color);
  color: white;
  padding: 0 0.75rem;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.tool-result {
  border-top: 1px solid var(--border-color);
  padding-top: 1.25rem;
}

.status-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.25rem;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
}

.action-buttons {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.btn-tool {
  width: 100%;
  padding: 0.6rem;
  background: white;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  text-align: center;
}

.btn-tool:hover {
  background: var(--bg-color);
  border-color: var(--text-tertiary);
  color: var(--text-primary);
}

.btn-download {
  color: var(--accent-color);
  border-color: var(--accent-soft);
  background: var(--accent-soft);
}

.btn-download:hover {
  background: rgba(37, 99, 235, 0.1);
}

.error-msg {
  font-size: 12px;
  color: var(--status-failed);
  background: rgba(239, 68, 68, 0.05);
  padding: 0.75rem;
  border-radius: 8px;
}

.empty-state {
  font-size: 12px;
  color: var(--text-tertiary);
  text-align: center;
  padding: 1rem 0;
}

.loader {
  font-size: 12px;
  color: var(--accent-color);
  text-align: center;
  animation: breathe 1.5s infinite;
}

@keyframes breathe {
  0% { opacity: 0.5; }
  50% { opacity: 1; }
  100% { opacity: 0.5; }
}
</style>
