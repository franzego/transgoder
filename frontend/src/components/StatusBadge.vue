<script setup>
const props = defineProps({
  status: {
    type: String,
    required: true
  }
})

const label = {
  pending: 'Pending',
  uploading: 'Uploading',
  queued: 'Queued',
  processing: 'Processing',
  downloading: 'Readying',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled'
}
</script>

<template>
  <span class="status-badge" :class="status">
    {{ label[status] || status }}
  </span>
</template>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  padding: 0.25rem 0.6rem;
  border-radius: 99px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.pending { background: rgba(251, 191, 36, 0.1); color: #b45309; }
.uploading { background: var(--accent-soft); color: var(--accent-color); }
.queued { background: rgba(107, 114, 128, 0.1); color: #4b5563; }
.processing { background: rgba(59, 130, 246, 0.1); color: #1d4ed8; }
.downloading { background: rgba(139, 92, 246, 0.1); color: #6d28d9; }
.completed { background: rgba(16, 185, 129, 0.1); color: #047857; }
.failed { background: rgba(239, 68, 68, 0.1); color: #b91c1c; }
.cancelled { background: rgba(107, 114, 128, 0.1); color: #374151; }
</style>
