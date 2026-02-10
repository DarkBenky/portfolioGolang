# Environment Configuration

This project uses environment variables to switch between development and production modes.

## Configuration Files

### Backend (.env in root directory)
```
DEV_MODE=true

BACKEND_GO_PORT=8085
BACKEND_PYTHON_PORT=5123
SERVER_HOST=91.98.145.193

SALT="your-salt-here"
JWT_SECRET="your-jwt-secret-here"
```

### Frontend (.env in frontend directory)
```
VITE_DEV_MODE=true
VITE_SERVER_HOST=91.98.145.193
VITE_BACKEND_PORT=8085
```

## Modes

### Development Mode (DEV_MODE=true)
- Backend connects to Python API at `http://localhost:5123/api`
- Frontend connects to Go backend at `http://localhost:8085`
- CORS allows `http://localhost:5173`
- All services run on localhost

### Production Mode (DEV_MODE=false)
- Backend connects to Python API at `http://{SERVER_HOST}:5123/api`
- Frontend connects to Go backend at `http://{SERVER_HOST}:8085`
- CORS allows `http://{SERVER_HOST}:5173`
- All services use the external server IP

## Switching Between Modes

### To run locally (Development):
1. Set `DEV_MODE=true` in root `.env`
2. Set `VITE_DEV_MODE=true` in `frontend/.env`
3. Access frontend at `http://localhost:5173`

### To run on remote server (Production):
1. Set `DEV_MODE=false` in root `.env`
2. Set `VITE_DEV_MODE=false` in `frontend/.env`
3. Update `SERVER_HOST` to your server IP
4. Access frontend at `http://{SERVER_HOST}:5173`

## Starting Services

### Backend (Go)
```bash
./main
```

### Backend (Python)
```bash
./start_python_server.sh
```
or manually:
```bash
python getData.py
```

### Frontend
```bash
cd frontend
npm run dev
```
