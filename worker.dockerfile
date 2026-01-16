FROM golang:1.24.3-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o worker ./cmd/worker

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/worker .

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

CMD ["./worker"]