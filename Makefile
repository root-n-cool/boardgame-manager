.PHONY: backend-test backend-build frontend-build build run docker-build

backend-test:
	cd backend && go test ./...

backend-build:
	cd backend && CGO_ENABLED=0 go build -o ../bin/server ./cmd/server

frontend-build:
	cd frontend && npm install && npm run build

build: frontend-build backend-build

run:
	cd backend && go run ./cmd/server

docker-build:
	docker compose build
