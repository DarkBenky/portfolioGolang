const devMode = import.meta.env.VITE_DEV_MODE === 'true'
const serverHost = import.meta.env.VITE_SERVER_HOST
const backendPort = import.meta.env.VITE_BACKEND_PORT
const pythonPort = import.meta.env.VITE_PYTHON_PORT || '5123'

export const API_BASE_URL = devMode 
  ? `http://localhost:${backendPort}`
  : `http://${serverHost}:${backendPort}`

export const PYTHON_API_URL = devMode
  ? `http://localhost:${pythonPort}`
  : `http://${serverHost}:${pythonPort}`
