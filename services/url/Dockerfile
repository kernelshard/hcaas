# --- Stage 1: Build ---
FROM golang:1.24.5-alpine3.22 AS builder

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /app

# Copy go.mod and go.sum to cache dependencies
COPY services/url/go.mod services/url/go.sum ./

# Copy pkg directory for local dependencies
COPY pkg/ ../pkg/

# Copy all source code
COPY services/url/ .

RUN go mod download
RUN go mod tidy

# Build binary from cmd/url
RUN go build -o hcaas ./cmd/url

# --- Stage 2: Minimal runtime ---
FROM gcr.io/distroless/static-debian11:nonroot

WORKDIR /

COPY --from=builder /app/hcaas .

EXPOSE 8080

ENTRYPOINT ["/hcaas"]
