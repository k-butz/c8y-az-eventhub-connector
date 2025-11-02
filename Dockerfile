FROM --platform=${BUILDPLATFORM} golang:latest AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY . .
RUN mkdir -p /etc/az-eventhub-connector
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -a -installsuffix cgo -o app main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /go/bin
COPY --from=builder /app/app .
COPY --from=builder /app/config.toml /etc/az-eventhub-connector/config.toml
COPY --from=builder /app/banner.txt ./banner.txt
CMD ["./app"]