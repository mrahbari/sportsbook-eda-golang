#!/bin/bash

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "🛑 Stopping Sportsbook EDA Stack..."

# Run from project root
cd "$PROJECT_ROOT"
docker compose stop

echo "✅ System stopped. Containers remain (un-paused)."
