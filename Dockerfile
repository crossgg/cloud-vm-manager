FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o cloud-vm-manager .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app/

COPY --from=builder /app/cloud-vm-manager .
COPY --from=builder /app/public ./public
RUN mkdir -p /app/keys

EXPOSE 3000

CMD ["./cloud-vm-manager"]
