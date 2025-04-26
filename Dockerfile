# Use the Go version you specified
FROM golang:1.23.3-alpine

WORKDIR /app

# Copy source code first for caching Go modules
COPY go.mod go.sum ./
# Optional: Download dependencies if not vendored
RUN apk add --no-cache git # git might be needed for go mod download
RUN go mod download

# Copy the rest of the source code
COPY . .

# --- Add Docker CLI ---
RUN apk add --no-cache curl docker-cli
# --- End Add Docker CLI ---

# Build the binaries (as per your original file)
RUN go build -o node main.go database.go tree.go multicast.go
RUN go build -o middleware middleware.go
RUN go build -o membership membership.go

# Expose necessary ports (keep existing ones)
EXPOSE 8080 8090 7946 7946/udp

# CMD is set in docker-compose 'command' directive
