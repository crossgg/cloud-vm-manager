FROM golang:1.22-alpine AS builder
ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -a -installsuffix cgo -o cloud-vm-manager .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app/

COPY --from=builder /app/cloud-vm-manager .
COPY --from=builder /app/public ./public
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN mkdir -p /app/config/keys /app/runtime && chmod +x /app/docker-entrypoint.sh

EXPOSE 3000

ENTRYPOINT ["/app/docker-entrypoint.sh"]
