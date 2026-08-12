#!/bin/bash

cd "$(dirname "$0")"

ulimit -n 65536

CONDA_SH="/home/user/anaconda3/etc/profile.d/conda.sh"
if [ -f "$CONDA_SH" ]; then
    source "$CONDA_SH"
    conda activate portfolio
elif [ -f "venv/bin/activate" ]; then
    source venv/bin/activate
fi

if ! command -v gunicorn &>/dev/null; then
    pip install gunicorn
fi

if ! python -c "import duckduckgo_search" 2>/dev/null; then
    echo "[$(date)] Installing duckduckgo_search..."
    pip install duckduckgo_search
fi

set -a
source .env 2>/dev/null || true
set +a

PORT="${BACKEND_PYTHON_PORT:-5123}"
HEALTH_URL="http://127.0.0.1:$PORT/api/health"
STARTUP_TIMEOUT="${PYTHON_STARTUP_TIMEOUT:-120}"

echo "Starting Python backend with auto-restart..."

while true; do
    rm -f gunicorn.ctl
    echo "[$(date)] Starting gunicorn on port $PORT"
    gunicorn -w 1 --threads 10 --timeout 120 --keep-alive 5 -b 127.0.0.1:"$PORT" getData:app &
    GUNICORN_PID=$!

    HEALTHY=0
    ELAPSED=0
    while [ "$ELAPSED" -lt "$STARTUP_TIMEOUT" ]; do
        if ! kill -0 "$GUNICORN_PID" 2>/dev/null; then
            echo "[$(date)] gunicorn exited during startup"
            break
        fi
        if curl -s -m 3 -o /dev/null "$HEALTH_URL" 2>/dev/null; then
            HEALTHY=1
            echo "[$(date)] Python API is healthy"
            break
        fi
        sleep 2
        ELAPSED=$((ELAPSED + 2))
    done

    if [ "$HEALTHY" -eq 0 ]; then
        echo "[$(date)] Python API did not become healthy within ${STARTUP_TIMEOUT}s, restarting"
        kill -9 "$GUNICORN_PID" 2>/dev/null
        wait "$GUNICORN_PID" 2>/dev/null
        sleep 3
        continue
    fi

    wait "$GUNICORN_PID"
    EXIT_CODE=$?
    echo "[$(date)] gunicorn exited with code $EXIT_CODE"

    if [ "$EXIT_CODE" -eq 0 ]; then
        echo "Clean exit, stopping auto-restart"
        break
    fi

    echo "Restarting in 3 seconds..."
    sleep 3
done
