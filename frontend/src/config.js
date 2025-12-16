const devMode = import.meta.env.VITE_DEV_MODE === 'true'
const serverHost = import.meta.env.VITE_SERVER_HOST
const backendPort = import.meta.env.VITE_BACKEND_PORT

export const API_BASE_URL = devMode 
  ? `http://localhost:${backendPort}`
  : `http://${serverHost}:${backendPort}`
