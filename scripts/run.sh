#!/bin/bash
set -e

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "🚀 Starting Sportsbook EDA Stack..."

# Run from project root to ensure docker-compose.yml is found
cd "$PROJECT_ROOT"
docker compose up --build -d

echo "--------------------------------------------------------"
echo "✅ System is coming up!"
echo "Gateway: http://localhost:8080"
echo "RabbitMQ UI: http://localhost:15672 (guest/guest)"
echo "--------------------------------------------------------"
echo "💡 Hint: Use 'scripts/stop.sh' to halt or 'docker compose logs -f' to follow."
