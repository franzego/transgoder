# Transcoder Frontend

A minimalist, modern Vue 3 single-page application for the Transcoder engine.

## Features

- **Modern UI**: Clean, neutral palette with subtle motion and strong spacing.
- **Multipart Upload**: Efficiently uploads large video files in parts directly to storage via presigned URLs.
- **Real-time Status**: Polling mechanism to track transcoding progress.
- **Job Tools**: Quick access to source, output, and download streams for any job ID.
- **Responsive**: Fully functional on mobile and desktop.

## Tech Stack

- **Framework**: Vue 3 (Composition API)
- **State Management**: Pinia
- **Build Tool**: Vite
- **Styling**: Plain CSS with CSS Variables (No Tailwind)
- **Routing**: Vue Router

## Getting Started

### Prerequisites

- Node.js (v18+)
- npm or yarn
- Backend running on `http://localhost:8084`

### Installation

```bash
cd frontend
npm install
```

### Development

Run the development server with HMR:

```bash
npm run dev
```

The app will be available at `http://localhost:3000`. API calls are proxied to the backend.

### Build

Compile and minify for production:

```bash
npm run build
```

## API Assumptions

The frontend expects the following endpoints from the backend:

- `POST /upload/initiate`: Initialize multipart upload and receive presigned URLs.
- `POST /upload/complete`: Finalize upload and provide video metadata.
- `GET /status/:id/update`: Retrieve current job status.
- `POST /status/:id/update`: Update job status (e.g., cancel).
- `GET /jobs/:id/source-url`: Get presigned URL for the source file.
- `GET /jobs/:id/output-url`: Get presigned URL for the transcoded file.
- `GET /jobs/:id/download`: Direct stream of the output video.

## Visual Design Tokens

Colors and spacing are controlled via `:root` variables in `src/style.css`. 
Key accent color: `#2563eb` (Restrained Blue).
