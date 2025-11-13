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