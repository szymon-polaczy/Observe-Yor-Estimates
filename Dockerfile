# Dockerfile for Go application

# --- Build Stage ---
FROM golang:1.18-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# We want to populate the module cache based on the go.mod file.
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go app
# -ldflags="-w -s" reduces the size of the binary
# CGO_ENABLED=0 is important for a static build
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o observe-yor-estimates .

# --- Deploy Stage ---
FROM alpine:latest

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the pre-built binary from the previous stage
COPY --from=builder /app/observe-yor-estimates .

# Add a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["./observe-yor-estimates"] 