/**
 * API Service for Transcoder
 */

const BASE_URL = ''; // Proxied by Vite

export const api = {
  /**
   * Initiate multipart upload
   */
  async initiateUpload({ fileName, fileSize, partSize }) {
    const response = await fetch(`${BASE_URL}/upload/initiate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ file_name: fileName, file_size: fileSize, part_size: partSize })
    });
    if (!response.ok) throw new Error(await getErrorMessage(response));
    return response.json();
  },

  /**
   * Upload a part to a presigned URL
   */
  async uploadPart(url, blob, onProgress) {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('PUT', url);
      
      if (onProgress) {
        xhr.upload.onprogress = (event) => {
          if (event.lengthComputable) {
            onProgress(event.loaded, event.total);
          }
        };
      }

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          const etag = xhr.getResponseHeader('ETag');
          resolve({ etag: etag ? etag.replace(/"/g, '') : '' });
        } else {
          reject(new Error(`Part upload failed with status ${xhr.status}`));
        }
      };
      
      xhr.onerror = () => reject(new Error('Part upload network error'));
      xhr.send(blob);
    });
  },

  /**
   * Complete multipart upload
   */
  async completeUpload(payload) {
    const response = await fetch(`${BASE_URL}/upload/complete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    if (!response.ok) throw new Error(await getErrorMessage(response));
    return response.json();
  },

  /**
   * Get job status
   */
  async getStatus(jobId) {
    // Note: backend route is /status/:id/update for GET too based on research
    const response = await fetch(`${BASE_URL}/status/${jobId}/update`);
    if (!response.ok) throw new Error(await getErrorMessage(response));
    return response.json();
  },

  /**
   * Update job status (cancel)
   */
  async updateStatus(jobId, from, to) {
    const response = await fetch(`${BASE_URL}/status/${jobId}/update`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: jobId, from, to })
    });
    if (!response.ok) throw new Error(await getErrorMessage(response));
    return response.json();
  },

  /**
   * Get source URL
   */
  async getSourceUrl(jobId) {
    const response = await fetch(`${BASE_URL}/jobs/${jobId}/source-url`);
    if (!response.ok) throw new Error(await getErrorMessage(response));
    return response.json();
  },

  /**
   * Get output URL
   */
  async getOutputUrl(jobId) {
    const response = await fetch(`${BASE_URL}/jobs/${jobId}/output-url`);
    if (!response.ok) throw new Error(await getErrorMessage(response));
    return response.json();
  },

  /**
   * Download output stream
   */
  getDownloadUrl(jobId) {
    return `${BASE_URL}/jobs/${jobId}/download`;
  }
};

async function getErrorMessage(response) {
  try {
    const data = await response.json();
    return data.message || data.error || `HTTP error! status: ${response.status}`;
  } catch (e) {
    return `HTTP error! status: ${response.status}`;
  }
}
