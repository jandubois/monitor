# Build frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web/frontend
COPY web/frontend/package*.json ./
RUN npm ci
COPY web/frontend/ ./
RUN npm run build

# Build Go binary (includes all built-in probes)
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/frontend/dist ./internal/web/frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o monitor .

# Final image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata git
WORKDIR /app
COPY --from=go-builder /app/monitor .
ENTRYPOINT ["/app/monitor"]
