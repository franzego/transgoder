/**
 * API Service for Transcoder
 */

const BASE_URL = (import.meta.env.VITE_API_BASE_URL || '').replace(/\/$/, '')

export const api = {
  async initiateUpload({ fileName, fileSize, partSize }) {
    const response = await fetch(`${BASE_URL}/upload/initiate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ file_name: fileName, file_size: fileSize, part_size: partSize })
    })
    if (!response.ok) throw new Error(await getErrorMessage(response))
    return response.json()
  },

  async uploadPart(url, blob, onProgress) {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('PUT', url)

      if (onProgress) {
        xhr.upload.onprogress = (event) => {
          if (event.lengthComputable) {
            onProgress(event.loaded, event.total)
          }
        }
      }

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          const etag = xhr.getResponseHeader('ETag')
          resolve({ etag: etag ? etag.replace(/"/g, '') : '' })
        } else {
          reject(new Error(`Part upload failed with status ${xhr.status}`))
        }
      }

      xhr.onerror = () => reject(new Error('Part upload network error'))
      xhr.send(blob)
    })
  },

  async completeUpload(payload) {
    const response = await fetch(`${BASE_URL}/upload/complete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!response.ok) throw new Error(await getErrorMessage(response))
    return response.json()
  },

  async createJob(payload) {
    const response = await fetch(`${BASE_URL}/jobs`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!response.ok) throw new Error(await getErrorMessage(response))
    return response.json()
  },

  async getPresets() {
    const response = await fetch(`${BASE_URL}/presets`)
    if (!response.ok) throw new Error(await getErrorMessage(response))
    const data = await safeJson(response, 'presets')
    return normalizePresets(data)
  },

  async getPreset(id) {
    const response = await fetch(`${BASE_URL}/presets/${id}`)
    if (!response.ok) throw new Error(await getErrorMessage(response))
    const data = await response.json()
    return data.metadata
  },

  async getStatus(jobId) {
    const response = await fetch(`${BASE_URL}/status/${jobId}/update`)
    if (!response.ok) throw new Error(await getErrorMessage(response))
    return response.json()
  },

  async updateStatus(jobId, from, to) {
    const response = await fetch(`${BASE_URL}/status/${jobId}/update`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: jobId, from, to })
    })
    if (!response.ok) throw new Error(await getErrorMessage(response))
    return response.json()
  },

  async getSourceUrl(jobId) {
    const response = await fetch(`${BASE_URL}/jobs/${jobId}/source-url`)
    if (!response.ok) throw new Error(await getErrorMessage(response))
    return response.json()
  },

  async getOutputUrl(jobId) {
    const response = await fetch(`${BASE_URL}/jobs/${jobId}/output-url`)
    if (!response.ok) throw new Error(await getErrorMessage(response))
    return response.json()
  },

  getDownloadUrl(jobId) {
    return `${BASE_URL}/jobs/${jobId}/download`
  }
}

async function getErrorMessage(response) {
  try {
    const data = await response.json()
    return data.message || data.error || `HTTP error! status: ${response.status}`
  } catch (e) {
    return `HTTP error! status: ${response.status}`
  }
}

async function safeJson(response, endpointName) {
  const contentType = response.headers.get('content-type') || ''
  const body = await response.text()

  if (!body.trim()) {
    throw new Error(`Empty response from ${endpointName} endpoint`)
  }

  if (!contentType.toLowerCase().includes('application/json')) {
    throw new Error(`Expected JSON from ${endpointName} endpoint, got ${contentType || 'unknown content type'}`)
  }

  try {
    return JSON.parse(body)
  } catch {
    throw new Error(`Invalid JSON from ${endpointName} endpoint`)
  }
}

function normalizePresets(payload) {
  if (Array.isArray(payload)) {
    return payload
  }

  const candidateArrays = [
    payload?.metadata,
    payload?.data,
    payload?.presets,
    payload?.metadata?.presets,
    payload?.metadata?.items
  ]

  for (const candidate of candidateArrays) {
    if (Array.isArray(candidate)) {
      return candidate
    }
  }

  if (payload?.metadata && typeof payload.metadata === 'object' && !Array.isArray(payload.metadata)) {
    const values = Object.values(payload.metadata)
    if (values.length > 0 && values.every((item) => item && typeof item === 'object')) {
      return values
    }
  }

  return []
}
