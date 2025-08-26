set dotenv-load

run:
    go run main.go

build-microservice TAG="latest":
    rm -f az-eventhub-connector.zip
    docker buildx build --load --platform linux/amd64 -t az-eventhub-connector:{{TAG}} .
    docker save az-eventhub-connector:{{TAG}} > image.tar
    zip az-eventhub-connector.zip image.tar cumulocity.json

deploy:
    c8y microservices create --file ./az-eventhub-connector.zip