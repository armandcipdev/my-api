# Stage 1: Build
FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

# Build binary Go
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Stage 2: Run container
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/server .

# Cloud Run requires PORT env, default to 8080
ENV PORT=8080

EXPOSE 8080

CMD ["./server"]
