# Start from the latest golang base image
FROM --platform=${BUILDPLATFORM} golang:latest AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -a -installsuffix cgo -o app main.go

# output image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /go/bin
COPY --from=builder /app/app .
CMD ["./app"]