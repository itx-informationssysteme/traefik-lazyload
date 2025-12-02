# =========================
# BUILD (normal)
# =========================
FROM golang:1.24-alpine AS build

WORKDIR /opt/src
COPY go.* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o traefik-lazyload .


# =========================
# BUILD-DEBUG (with delve)
# =========================
FROM golang:1.24-alpine AS build-debug

# Install Delve
RUN apk add --no-cache gcc git libc-dev && \
    go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /opt/src
COPY go.* ./
RUN go mod download

COPY . .

# Build with NO optimizations so debugging works
# CGO_ENABLED=1 is required for Delve to properly map source files
RUN CGO_ENABLED=0 GOOS=linux go build \
    -buildvcs=false \
    -gcflags "all=-N -l" \
    -o traefik-lazyload .


# =========================
# PROD IMAGE (unchanged)
# =========================
FROM alpine:3.19 AS prod

RUN apk --no-cache add ca-certificates
WORKDIR /opt/app

COPY --from=build /opt/src/traefik-lazyload .
COPY config.yaml .

EXPOSE 8080
CMD ["./traefik-lazyload"]


# =========================
# DEBUG IMAGE (for vscode)
# =========================
FROM golang:1.24-alpine AS debug

RUN apk add --no-cache bash ca-certificates gcc git libc-dev && \
    go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /opt/src

COPY --from=build-debug /opt/src/traefik-lazyload /tmp/traefik-lazyload
COPY config.yaml .

EXPOSE 8080 40000

# Build the binary with debug info and run with dlv exec
CMD ["dlv", "exec", "/tmp/traefik-lazyload", "--headless", "--listen=:40000", "--api-version=2", "--accept-multiclient", "--continue", "--log"]
