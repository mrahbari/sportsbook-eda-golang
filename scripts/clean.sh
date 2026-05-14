#!/bin/bash

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "🧹 Cleaning Sportsbook EDA Environment..."

# Run from project root
cd "$PROJECT_ROOT"
docker compose down -v

# Remove the specific application images created by this project
echo "🗑️  Removing project-specific Docker images..."
docker images "learning-go-*" -q | xargs -r docker rmi -f

echo "✅ Clean complete. Database volumes and project images have been removed."
