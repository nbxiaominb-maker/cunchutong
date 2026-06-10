FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /bucketd ./cmd/bucketd/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bucketd /usr/local/bin/bucketd
COPY configs/bucketd.yaml /etc/bucketd/config.yaml

VOLUME /data
EXPOSE 8080

ENTRYPOINT ["bucketd"]
CMD ["--config", "/etc/bucketd/config.yaml"]
