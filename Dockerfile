# Stage 1: Build React 19 Cockpit UI
FROM node:22-alpine AS ui-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build single Go binary
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod ./
COPY internal/ ./internal/
COPY cmd/ ./cmd/
COPY --from=ui-builder /app/internal/ui/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -extldflags '-static'" -o /bin/sqlloom ./cmd/sqlloom

# Stage 3: Scratch final container (< 20MB)
FROM scratch
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go-builder /bin/sqlloom /sqlloom

EXPOSE 5432 8080
ENTRYPOINT ["/sqlloom"]
CMD ["-listen", ":5432", "-upstream", "postgres:5432", "-dashboard", ":8080"]
