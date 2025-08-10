# auth/Dockerfile
FROM golang:1.24.5-alpine

WORKDIR /app

# Enable Go modules and download deps
COPY go.mod go.sum ./
RUN go mod download

# Copy the whole app and build
COPY . .
RUN go build -o auth ./cmd/auth

EXPOSE 8081
CMD ["./auth"]
