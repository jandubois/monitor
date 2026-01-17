.PHONY: build build-frontend build-go build-probes run-watcher run-web migrate clean

# Build everything
build: build-frontend build-go build-probes

# Build frontend and copy to internal/web
build-frontend:
	cd web/frontend && npm ci && npm run build
	mkdir -p internal/web/frontend
	cp -r web/frontend/dist internal/web/frontend/

# Build Go binary (requires frontend to be built first)
build-go:
	go build -o monitor .

# Build probe executables
build-probes:
	go build -o probes/disk-space/disk-space ./probes/disk-space
	go build -o probes/command/command ./probes/command

# Run watcher locally (requires DATABASE_URL)
run-watcher:
	go run . watcher --probes-dir ./probes

# Run web server locally (requires DATABASE_URL and AUTH_TOKEN)
run-web:
	go run . web

# Run database migrations
migrate:
	go run . migrate

# Clean build artifacts
clean:
	rm -rf monitor
	rm -rf internal/web/frontend/dist
	rm -f probes/disk-space/disk-space
	rm -f probes/command/command

# Docker build
docker-build:
	docker compose build

# Docker up
docker-up:
	docker compose up -d

# Docker down
docker-down:
	docker compose down
