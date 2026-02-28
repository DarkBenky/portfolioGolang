#!/bin/bash

cd "$(dirname "$0")"

ulimit -n 65536

if [ -f "venv/bin/activate" ]; then
    source venv/bin/activate
fi

if ! command -v gunicorn &>/dev/null; then
    pip install gunicorn
fi

set -a
source .env 2>/dev/null || true
set +a

PORT="${BACKEND_PYTHON_PORT:-5123}"

echo "Starting Python backend with auto-restart..."

while true; do
    echo "[$(date)] Starting gunicorn on port $PORT"
    gunicorn -w 1 --threads 10 --timeout 120 --keep-alive 5 -b 0.0.0.0:"$PORT" getData:app

    EXIT_CODE=$?
    echo "[$(date)] gunicorn exited with code $EXIT_CODE"

    if [ $EXIT_CODE -eq 0 ]; then
        echo "Clean exit, stopping auto-restart"
        break
    fi

    echo "Restarting in 3 seconds..."
    sleep 3
done
