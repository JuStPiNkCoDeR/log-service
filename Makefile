# Copyright sasha.los.0148@gmail.com
# All Rights have been taken by Mafia :)
include .env
#export $(shell sed 's/=.*//' .env) Export all .env vars
###########################
# Basic params & commands #
###########################
SERVICE_PATH=$(PWD)
SERVICE_NAME=$(shell basename $(SERVICE_PATH))
LIB_PATH=$(SERVICE_PATH)/../../lib
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_CLEAN=$(GO_CMD) clean
GO_TEST=$(GO_CMD) test
GO_GET=$(GO_CMD) get
CONTAINER_WORKDIR=/go/src/app
CONTAINER_BUILD_NAME=$(SERVICE_NAME)-builder
CONFIG_PATH=${HOME}/.config
################
# Binary names #
################
BINARY_NAME=bin
BINARY_UNIX=$(BINARY_NAME)_unix
#########
# Tasks #
#########
.SILENT:
all: deps protobuf_compile init_ssl test
.PHONY: init_ssl
init_ssl:
	echo "Generating SSL certificates..."
	mkdir -p $(CONFIG_PATH)

	cfssl gencert \
		-initca tls/ca-csr.json | cfssljson -bare ca
	# Server certificate
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=tls/ca-config.json \
		-profile=server \
		tls/server-csr.json | cfssljson -bare server
	# Client certificates
	cfssl gencert \
    	-ca=ca.pem \
    	-ca-key=ca-key.pem \
    	-config=tls/ca-config.json \
    	-profile=client \
    	tls/client-csr.json | cfssljson -bare client
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=tls/ca-config.json \
		-profile=client \
		-cn="root" \
		tls/client-csr.json | cfssljson -bare root-client
	cfssl gencert \
		-ca=ca.pem \
        -ca-key=ca-key.pem \
        -config=tls/ca-config.json \
        -profile=client \
        -cn="nobody" \
        tls/client-csr.json | cfssljson -bare nobody-client

	cp tls/model.conf $(CONFIG_PATH)/model.conf
	cp tls/policy.csv $(CONFIG_PATH)/policy.csv
	mv *.pem *.csr $(CONFIG_PATH)
copy_lib:
	echo "Copying lib files to the service..."
	cp -r $(LIB_PATH) $(SERVICE_PATH)
	rm $(SERVICE_PATH)/lib/go.mod
	rm $(SERVICE_PATH)/lib/go.sum
protobuf_compile:
	echo "Proto files compilation..."
	protoc api/v1/*.proto \
    	--gogo_out=Mgogoproto/gogo.proto=github.com/gogo/protobuf/proto,plugins=grpc:. \
    	--proto_path=$$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf) \
    	--proto_path=.
build:
	echo "Building application..."
	$(GO_BUILD) -o $(BINARY_NAME) -v
test: $(CONFIG_PATH)/policy.csv $(CONFIG_PATH)/model.conf
	echo "Testing appication..."
	$(GO_TEST) -v ./...
clean:
	echo "Cleaing..."
	$(GO_CLEAN) ./...
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f *.pem
	rm -f *.csr
	rm -f api/**/*.pb.go
run:
	echo "Staring application..."
	./$(BINARY_NAME)
deps:
	echo "Installing dependencies..."
	$(GO_GET) -d -v ./...
# Cross-platform
lint-docker:
	echo "Linting in docker..."
	docker run --rm \
		-v $(SERVICE_PATH):/app \
		-w /app \
		golangci/golangci-lint:v1.30.0 \
		golangci-lint run -v
build-linux:
	echo "Building for linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BINARY_NAME) -v
build-docker:
	echo "Building in docker..."
	cp -r $(LIB_PATH) $(SERVICE_PATH) \
    	&& rm $(SERVICE_PATH)/lib/go.mod \
    	&& rm $(SERVICE_PATH)/lib/go.sum
	docker build -t $(SERVICE_NAME) .
	docker run -td \
		--name $(CONTAINER_BUILD_NAME) $(SERVICE_NAME)
	docker cp $(CONTAINER_BUILD_NAME):$(CONTAINER_WORKDIR)/api $(SERVICE_PATH)
	docker rm -f $(CONTAINER_BUILD_NAME)
run-docker:
	# Copy build binary and run it