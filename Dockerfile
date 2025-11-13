# BUILD
FROM golang:1.24-alpine AS build

WORKDIR /opt/src
COPY go.* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o traefik-lazyload .

# Make the final image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates
WORKDIR /opt/app

# Copy the binary from the build stage
COPY --from=build /opt/src/traefik-lazyload .
COPY config.yaml .

EXPOSE 8080

# Run as non-root user
RUN adduser -D -s /bin/sh appuser
USER appuser

CMD ["./traefik-lazyload"]
