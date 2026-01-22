run:
    go run ./cmd/api

swagger:
    go install github.com/swaggo/swag/cmd/swag@latest
    $(go env GOPATH)/bin/swag init -g cmd/api/main.go --parseDependency --parseInternal

start:
    $(go env GOPATH)/bin/swag init -g cmd/api/main.go --parseDependency --parseInternal
    go run ./cmd/api

build:
    go build -o app-busca-search ./cmd/api

tidy:
    go mod tidy

test:
    go test ./...

# Frontend dev server
frontend:
    cd frontend && npm run dev

# Reindex commands
reindex-all:
    go run cmd/reindex/main.go --collection=prefrio_services_base --mode=all

reindex-missing:
    go run cmd/reindex/main.go --collection=prefrio_services_base --missing-only

reindex-dry:
    go run cmd/reindex/main.go --collection=prefrio_services_base --mode=all --dry-run

reindex-content:
    go run cmd/reindex/main.go --collection=prefrio_services_base --mode=content-only

reindex-embeddings:
    go run cmd/reindex/main.go --collection=prefrio_services_base --mode=embedding-only