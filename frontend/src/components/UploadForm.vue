<script setup>
import { ref, reactive } from 'vue'
import { useJobStore } from '../stores/jobStore'
import { api } from '../services/api'

const jobStore = useJobStore()

const fileInput = ref(null)
const isDragging = ref(false)
const isUploading = ref(false)

const form = reactive({
  file: null,
  videoName: '',
  description: '',
  format: 'mp4',
  codec: 'h264',
  resolution: '1080',
  framerate: 30,
  duration: 0,
  partSize: 64, // MB
  maxRetries: 3,
  partTimeout: 30 // Seconds
})

const handleFileChange = (e) => {
  const selected = e.target.files[0]
  if (selected) setFile(selected)
}

const handleDrop = (e) => {
  isDragging.value = false
  const selected = e.dataTransfer.files[0]
  if (selected) setFile(selected)
}

const setFile = (file) => {
  form.file = file
  if (!form.videoName) {
    form.videoName = file.name.split('.').slice(0, -1).join('.')
  }
}

const triggerFileInput = () => {
  fileInput.value.click()
}

const startUpload = async () => {
  if (!form.file) return
  
  const tempId = `upload_${Date.now()}`
  const videoName = form.videoName
  const uploadFile = form.file
  
  // Add to store immediately
  jobStore.addJob({
    id: tempId,
    name: videoName,
    status: 'pending',
    progress: 0
  })

  // Local copy of config to prevent changes while uploading
  const config = { ...form }
  
  // Reset form for next upload
  form.file = null
  form.videoName = ''
  form.description = ''

  try {
    // 1. Initiate
    const initRes = await api.initiateUpload({
      fileName: uploadFile.name,
      fileSize: uploadFile.size,
      partSize: config.partSize * 1024 * 1024
    })
    
    const { job_id, upload_id, parts, part_size } = initRes.metadata
    
    // Update store with real job_id, but we might need to handle the ID change
    // For simplicity, we keep the job in the same slot but update its real ID
    jobStore.activeJobs[tempId].id = job_id
    jobStore.updateJob(tempId, { status: 'uploading' })

    // 2. Upload parts
    const completedParts = []
    const totalParts = parts.length
    
    for (const part of parts) {
      const start = (part.part_number - 1) * part_size
      const end = Math.min(start + part_size, uploadFile.size)
      const blob = uploadFile.slice(start, end)
      
      let retryCount = 0
      let success = false
      
      while (!success && retryCount <= config.maxRetries) {
        try {
          const { etag } = await api.uploadPart(part.url, blob)
          completedParts.push({ part_number: part.part_number, etag })
          success = true
          
          const progress = Math.round((completedParts.length / totalParts) * 100)
          jobStore.updateJob(tempId, { progress })
        } catch (err) {
          retryCount++
          if (retryCount > config.maxRetries) throw err
          await new Promise(r => setTimeout(r, 1000 * retryCount))
        }
      }
    }

    // 3. Complete
    await api.completeUpload({
      job_id,
      upload_id,
      parts: completedParts,
      video_name: videoName,
      description: config.description,
      format: config.format,
      resolution: config.resolution,
      codec: config.codec,
      framerate: parseInt(config.framerate),
      duration: parseInt(config.duration)
    })

    jobStore.updateJob(tempId, { status: 'queued', progress: 55 })
    // Move from tempId to real job_id in store for polling consistency
    const jobData = { ...jobStore.activeJobs[tempId], id: job_id }
    delete jobStore.activeJobs[tempId]
    jobStore.activeJobs[job_id] = jobData
    
    jobStore.startPolling(job_id)

  } catch (err) {
    jobStore.updateJob(tempId, { error: err.message, status: 'failed' })
  }
}
</script>

<template>
  <div class="upload-card">
    <div 
      class="drop-zone"
      :class="{ 'is-dragging': isDragging }"
      @dragover.prevent="isDragging = true"
      @dragleave.prevent="isDragging = false"
      @drop.prevent="handleDrop"
      @click="triggerFileInput"
    >
      <input 
        type="file" 
        ref="fileInput" 
        class="hidden" 
        @change="handleFileChange"
        accept="video/*"
      />
      
      <div v-if="!form.file" class="drop-content">
        <div class="drop-icon">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
            <polyline points="17 8 12 3 7 8"></polyline>
            <line x1="12" y1="3" x2="12" y2="15"></line>
          </svg>
        </div>
        <h3>Select a video file</h3>
        <p>Drag and drop or click to browse</p>
      </div>
      
      <div v-else class="selected-file">
        <div class="file-info">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"></path>
            <polyline points="14 2 14 8 20 8"></polyline>
          </svg>
          <span class="file-name">{{ form.file.name }}</span>
          <span class="file-size">({{ (form.file.size / (1024 * 1024)).toFixed(2) }} MB)</span>
        </div>
        <button class="btn-change" @click.stop="triggerFileInput">Change</button>
      </div>
    </div>

    <div class="form-grid">
      <div class="form-group full-width">
        <label>Video Name</label>
        <input type="text" v-model="form.videoName" placeholder="My Awesome Video" />
      </div>

      <div class="form-group full-width">
        <label>Description (Optional)</label>
        <textarea v-model="form.description" placeholder="A brief description of the content..."></textarea>
      </div>

      <div class="form-group">
        <label>Format</label>
        <select v-model="form.format">
          <option value="mp4">MP4</option>
          <option value="mov">MOV</option>
        </select>
      </div>

      <div class="form-group">
        <label>Codec</label>
        <select v-model="form.codec">
          <option value="h264">H.264 (AVC)</option>
          <option value="h265">H.265 (HEVC)</option>
        </select>
      </div>

      <div class="form-group">
        <label>Resolution</label>
        <select v-model="form.resolution">
          <option value="1080">1080p</option>
          <option value="720">720p</option>
          <option value="480">480p</option>
        </select>
      </div>

      <div class="form-group">
        <label>Framerate (FPS)</label>
        <input type="number" v-model="form.framerate" min="1" max="120" />
      </div>

      <div class="advanced-toggle" @click="showAdvanced = !showAdvanced">
        <span>Advanced Settings</span>
        <svg :class="{ 'rotated': showAdvanced }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"></polyline></svg>
      </div>

      <template v-if="showAdvanced">
        <div class="form-group">
          <label>Part Size (MB)</label>
          <input type="number" v-model="form.partSize" min="5" />
        </div>
        <div class="form-group">
          <label>Max Retries</label>
          <input type="number" v-model="form.maxRetries" min="0" />
        </div>
        <div class="form-group">
          <label>Part Timeout (s)</label>
          <input type="number" v-model="form.partTimeout" min="5" />
        </div>
        <div class="form-group">
          <label>Duration (s)</label>
          <input type="number" v-model="form.duration" min="0" />
        </div>
      </template>
    </div>

    <button 
      class="btn-primary" 
      :disabled="!form.file || isUploading"
      @click="startUpload"
    >
      <span v-if="!isUploading">Start Transcoding</span>
      <span v-else>Preparing Upload...</span>
    </button>
  </div>
</template>

<script>
// Non-setup part for simple data
export default {
  data() {
    return {
      showAdvanced: false
    }
  }
}
</script>

<style scoped>
.upload-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius);
  padding: 2rem;
  box-shadow: var(--shadow-md);
  display: flex;
  flex-direction: column;
  gap: 2rem;
}

.drop-zone {
  border: 2px dashed var(--border-color);
  border-radius: var(--border-radius);
  padding: 3rem 2rem;
  text-align: center;
  cursor: pointer;
  transition: all var(--transition-speed) ease;
  background-color: var(--bg-color);
}

.drop-zone:hover, .drop-zone.is-dragging {
  border-color: var(--accent-color);
  background-color: var(--accent-soft);
}

.drop-icon {
  color: var(--text-tertiary);
  margin-bottom: 1rem;
}

.drop-zone h3 {
  font-size: 16px;
  font-weight: 500;
  margin-bottom: 0.25rem;
}

.drop-zone p {
  font-size: 14px;
  color: var(--text-secondary);
}

.selected-file {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem;
}

.file-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-size: 14px;
}

.file-name {
  font-weight: 500;
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-size {
  color: var(--text-secondary);
}

.btn-change {
  background: transparent;
  color: var(--accent-color);
  font-weight: 500;
  font-size: 13px;
}

.hidden {
  display: none;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1.5rem;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.full-width {
  grid-column: span 2;
}

label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
}

input, select, textarea {
  padding: 0.75rem;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--bg-color);
  color: var(--text-primary);
  transition: border-color var(--transition-speed);
}

input:focus, select:focus, textarea:focus {
  outline: none;
  border-color: var(--accent-color);
}

textarea {
  min-height: 80px;
  resize: vertical;
}

.advanced-toggle {
  grid-column: span 2;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-tertiary);
  cursor: pointer;
  margin-top: 0.5rem;
}

.advanced-toggle svg {
  transition: transform var(--transition-speed);
}

.advanced-toggle svg.rotated {
  transform: rotate(180deg);
}

.btn-primary {
  background: var(--accent-color);
  color: white;
  padding: 1rem;
  border-radius: var(--border-radius);
  font-weight: 600;
  font-size: 15px;
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.2);
}

.btn-primary:hover:not(:disabled) {
  background: var(--accent-hover);
  transform: translateY(-1px);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

@media (max-width: 600px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .full-width {
    grid-column: span 1;
  }
  .advanced-toggle {
    grid-column: span 1;
  }
}
</style>
