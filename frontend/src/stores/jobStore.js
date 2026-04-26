import { defineStore } from 'pinia'
import { api } from '../services/api'

export const useJobStore = defineStore('job', {
  state: () => ({
    activeJobs: {}, // Map of jobId -> { id, name, status, progress, error }
    pollingIntervals: {} // Map of jobId -> intervalId
  }),

  getters: {
    jobList: (state) => Object.values(state.activeJobs).reverse(),
    hasActiveJobs: (state) => Object.keys(state.activeJobs).length > 0
  },

  actions: {
    addJob(job) {
      // For initial upload where ID might not be known yet, use a temp key
      const key = job.id || `temp_${Date.now()}`
      this.activeJobs[key] = {
        id: job.id || null,
        name: job.name,
        status: job.status || 'pending',
        progress: job.progress || 0,
        error: null,
        ...job
      }
      return key
    },

    updateJob(id, updates) {
      if (this.activeJobs[id]) {
        this.activeJobs[id] = { ...this.activeJobs[id], ...updates }
      }
    },

    removeJob(id) {
      this.stopPolling(id)
      delete this.activeJobs[id]
    },

    async startPolling(jobId) {
      if (this.pollingIntervals[jobId]) return
      
      const poll = async () => {
        try {
          const res = await api.getStatus(jobId)
          const newStatus = res.metadata.status
          this.updateJob(jobId, { status: newStatus })

          if (['completed', 'failed', 'cancelled'].includes(newStatus)) {
            this.stopPolling(jobId)
          }
        } catch (err) {
          console.error(`Polling error for ${jobId}:`, err)
        }
      }

      await poll()
      this.pollingIntervals[jobId] = setInterval(poll, 3000)
    },

    stopPolling(jobId) {
      if (this.pollingIntervals[jobId]) {
        clearInterval(this.pollingIntervals[jobId])
        delete this.pollingIntervals[jobId]
      }
    },

    async cancelJob(jobId) {
      const job = this.activeJobs[jobId]
      if (!job || !job.id) return
      
      try {
        await api.updateStatus(job.id, job.status, 'cancelled')
        this.updateJob(jobId, { status: 'cancelled' })
        this.stopPolling(jobId)
      } catch (err) {
        this.updateJob(jobId, { error: `Cancellation failed: ${err.message}` })
      }
    }
  }
})
