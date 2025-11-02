set dotenv-load

svcName := "az-eventhub-connector"

# Build an executable for a given OS and CPU platform
build OS="darwin" ARCH="arm64":
    CGO_ENABLED=0 OS=darwin ARCH=arm64 go build -o {{svcName}}-{{OS}}-{{ARCH}} main.go

# Create an executable for all OS and ARCH combinations. The artifacts will be placed in /dist folder
# requires https://goreleaser.com/intro/ being installed
build-all:
    goreleaser release --snapshot --clean

# creates a ready-to-upload zip file in current directory
build-image ARCH="amd64" TAG="latest":
    CGO_ENABLED=0 GOOS=linux GOARCH={{ARCH}} go build -o {{svcName}} main.go
    docker buildx build --platform linux/{{ARCH}} -t {{svcName}}:{{TAG}} .
    docker save {{svcName}}:{{TAG}} > image.tar
    zip {{svcName}}.zip image.tar cumulocity.json

# runs your image locally, needs to be executed after build-image
run-image TAG="latest":
    docker run --env-file=.env {{svcName}}:{{TAG}}

# build the service as docker-image & upoad to Cumulocity
build-and-deploy-c8y: build-image deploy-ms

# upload and start a service (as zip) to Cumulocity
deploy-ms:
    c8y microservices create --file {{svcName}}.zip -f

# delete the associated service in Cumulocity
delete-ms:
    c8y microservices delete --id {{svcName}} -f