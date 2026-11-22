const devMode = import.meta.env.VITE_DEV_MODE === 'true'
const backendPort = import.meta.env.VITE_BACKEND_PORT || '8085'

// In production the Go server serves this bundle and the API from the same origin, so
// an empty base keeps requests relative and removes the need for CORS entirely.
export const API_BASE_URL = devMode ? `http://localhost:${backendPort}` : ''
