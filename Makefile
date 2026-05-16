.PHONY: build test run stop clean seed verify help

# Build all binaries
build:
	go build ./...

# Run all tests
test:
	go test -v ./...

# Start the full stack with CDC enabled
run:
	./scripts/run.sh

# Stop the stack
stop:
	./scripts/stop.sh

# Clean up docker volumes and local binaries
clean:
	./scripts/clean.sh

# Seed initial odds data
seed:
	./scripts/seed_odds.sh

# Run automated verification
verify:
	./scripts/verification.sh

# Follow logs for a specific service (e.g. make logs svc=bet-service)
logs:
	docker compose logs -f $(svc)

# Run migrations manually (requires stack to be up)
migrate:
	docker compose run --rm migrate

# Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build    Build all Go binaries"
	@echo "  test     Run all unit tests"
	@echo "  run      Start Docker stack (CDC mode)"
	@echo "  stop     Stop Docker stack"
	@echo "  clean    Wipe Docker volumes and local builds"
	@echo "  seed     Seed random odds to the system"
	@echo "  verify   Run end-to-end verification script"
	@echo "  logs     Follow logs (use svc=name)"
	@echo "  migrate  Run DB migrations manually"
