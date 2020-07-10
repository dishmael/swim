# Go parameters
GOCMD=go
PBCMD=protoc
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GORUN=$(GOCMD) run
RUNFILE=main/main.go
BINARY_NAME=swim
#BINARY_UNIX=$(BINARY_NAME)_unix
PROTO_FILE=rpc/rpc.proto

all: test build
build:
	$(GOBUILD) -o $(BINARY_NAME) -v
proto:
	$(PBCMD) --go_out=plugins=grpc:. --go_opt=paths=source_relative ${PROTO_FILE}
test: 
	$(GOTEST) -v ./...
clean: 
	$(GOCLEAN)
	#rm -f $(BINARY_NAME)
	#rm -f $(BINARY_UNIX)
run:
	$(GORUN) $(RUNFILE)
#deps:
#	$(GOGET) github.com/markbates/goth
#	$(GOGET) github.com/markbates/pop
	
	
# Cross compilation
#build-linux:
#	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v
#docker-build:
#	docker run --rm -it -v "$(GOPATH)":/go -w /go/src/bitbucket.org/rsohlich/makepost golang:latest go build -o "$(BINARY_UNIX)" -v
