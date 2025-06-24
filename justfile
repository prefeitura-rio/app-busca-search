run:
    go run ./cmd/api

swagger:
    swag init -g cmd/api/main.go --parseDependency --parseInternal

start:
    swag init -g cmd/api/main.go --parseDependency --parseInternal
    go run ./cmd/api

build:
    go build -o app-busca-search ./cmd/api

tidy:
    go mod tidy

test:
    go test ./...