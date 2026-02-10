#!/bin/bash

cd "$(dirname "$0")"

echo "Starting Python backend with auto-restart..."

while true; do
    echo "[$(date)] Starting getData.py"
    python getData.py
    
    EXIT_CODE=$?
    echo "[$(date)] getData.py exited with code $EXIT_CODE"
    
    if [ $EXIT_CODE -eq 0 ]; then
        echo "Clean exit, stopping auto-restart"
        break
    fi
    
    echo "Restarting in 3 seconds..."
    sleep 3
done
