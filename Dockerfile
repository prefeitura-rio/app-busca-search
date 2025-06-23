
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4 && \
    /go/bin/swag init --parseDependency --parseInternal --output ./docs

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-s -w" -o /app/busca ./cmd/api

FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    update-ca-certificates

WORKDIR /app

COPY --from=builder /app/busca ./busca

ENV GIN_MODE=release

CMD ["./busca"]
