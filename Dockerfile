# Build frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web/frontend
COPY web/frontend/package*.json ./
RUN npm ci
COPY web/frontend/ ./
RUN npm run build

# Build Go binary
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/frontend/dist ./internal/web/frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o monitor .

# Build probes
RUN CGO_ENABLED=0 GOOS=linux go build -o probes/disk-space/disk-space ./probes/disk-space
RUN CGO_ENABLED=0 GOOS=linux go build -o probes/command/command ./probes/command
RUN CGO_ENABLED=0 GOOS=linux go build -o probes/debug/debug ./probes/debug
RUN CGO_ENABLED=0 GOOS=linux go build -o probes/github/github ./probes/github

# Final image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /app/monitor .
COPY --from=go-builder /app/probes ./probes
ENTRYPOINT ["/app/monitor"]
