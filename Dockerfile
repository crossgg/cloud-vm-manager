FROM golang:1.24-alpine AS builder
ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X github.com/crossgg/cloud-vm-manager/internal/updater.Version=${VERSION}" -a -installsuffix cgo -o cloud-vm-manager .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -a -installsuffix cgo -o launcher ./cmd/launcher
RUN echo "${VERSION}-$(date +%s)" > /app/.image_id

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app/

COPY --from=builder /app/.image_id .
COPY --from=builder /app/cloud-vm-manager .
COPY --from=builder /app/launcher .
COPY --from=builder /app/public ./public
RUN mkdir -p /app/config/keys /app/runtime && chmod +x /app/launcher /app/cloud-vm-manager

ENV GIN_MODE=release

EXPOSE 3000

ENTRYPOINT ["/app/launcher"]
