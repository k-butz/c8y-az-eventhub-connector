FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /go/bin
COPY ./az-eventhub-connector ./app
CMD ["./app"]