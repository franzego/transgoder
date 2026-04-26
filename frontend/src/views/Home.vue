<script setup>
import { useJobStore } from '../stores/jobStore'
import UploadForm from '../components/UploadForm.vue'
import ProgressPanel from '../components/ProgressPanel.vue'
import JobTools from '../components/JobTools.vue'
import ResultCard from '../components/ResultCard.vue'

const jobStore = useJobStore()
</script>

<template>
  <div class="page-container">
    <header class="app-header">
      <div class="logo">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polygon points="23 7 16 12 23 17 23 7"></polygon>
          <rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect>
        </svg>
        <span>Transcoder</span>
      </div>
    </header>

    <main class="app-main">
      <div class="content-grid">
        <!-- Main Panel -->
        <section class="main-panel">
          <UploadForm />
        </section>

        <!-- Side Panel -->
        <aside class="side-panel">
          <div class="active-jobs-section" v-if="jobStore.hasActiveJobs">
            <h3>Active Jobs</h3>
            <div class="jobs-list">
              <transition-group name="fade">
                <div v-for="job in jobStore.jobList" :key="job.id" class="job-item">
                  <ProgressPanel :job="job" />
                  <ResultCard v-if="job.status === 'completed'" :job-id="job.id" />
                  
                  <div class="job-actions">
                    <button 
                      v-if="['pending', 'uploading', 'queued', 'processing'].includes(job.status)"
                      class="btn-mini-cancel"
                      @click="jobStore.cancelJob(job.id)"
                    >
                      Cancel
                    </button>
                    <button 
                      v-if="['completed', 'failed', 'cancelled'].includes(job.status)"
                      class="btn-mini-clear"
                      @click="jobStore.removeJob(job.id)"
                    >
                      Clear
                    </button>
                  </div>
                </div>
              </transition-group>
            </div>
          </div>

          <JobTools />
        </aside>
      </div>
    </main>
    
    <footer class="app-footer">
      <p>&copy; 2026 Transcoder Engine. Modern. Minimalist. Fast.</p>
    </footer>
  </div>
</template>

<style scoped>
.page-container {
  max-width: var(--container-width);
  margin: 0 auto;
  padding: 2rem 1rem;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.app-header {
  margin-bottom: 3rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.logo {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-weight: 600;
  font-size: 1.25rem;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.logo svg {
  color: var(--accent-color);
}

.app-main {
  flex: 1;
}

.content-grid {
  display: grid;
  grid-template-columns: 1fr 350px;
  gap: 2.5rem;
  align-items: start;
}

@media (max-width: 950px) {
  .content-grid {
    grid-template-columns: 1fr;
  }
  
  .side-panel {
    order: 2;
  }
}

.main-panel {
  display: flex;
  flex-direction: column;
  gap: 2rem;
}

.side-panel {
  display: flex;
  flex-direction: column;
  gap: 2rem;
}

.active-jobs-section h3 {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 1rem;
  color: var(--text-primary);
}

.jobs-list {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.job-item {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  position: relative;
}

.job-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.btn-mini-cancel, .btn-mini-clear {
  background: transparent;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  cursor: pointer;
  padding: 2px 4px;
}

.btn-mini-cancel { color: var(--status-failed); }
.btn-mini-clear { color: var(--text-tertiary); }

.app-footer {
  margin-top: 4rem;
  padding-top: 2rem;
  border-top: 1px solid var(--border-color);
  text-align: center;
  color: var(--text-tertiary);
  font-size: 12px;
}

/* Deeper component overrides for sidebar density */
:deep(.progress-panel) {
  padding: 1rem;
}
:deep(.result-card) {
  padding: 1rem;
}
:deep(.actions-grid) {
  grid-template-columns: 1fr 1fr 1fr;
  gap: 0.5rem;
}
:deep(.action-item) {
  padding: 0.5rem;
}
:deep(.action-icon) {
  width: 32px;
  height: 32px;
}
</style>
