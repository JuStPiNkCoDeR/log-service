# Image for test and build service
FROM golang:1.13

# Protoc(RPC protocol buffer) installation
# See https://grpc.io/docs/languages/go/quickstart/
RUN apt-get update && apt-get install -y protobuf-compiler && protoc --version
# Protoc for GoLang
ENV GO111MODULE=on
RUN go get github.com/golang/protobuf/protoc-gen-go
ENV PATH="${PATH}:$(go env GOPATH)/bin"
# Installing RPC/gRPC plugins
RUN go get github.com/gogo/protobuf/...@v1.3.1
# Installing CloudFlare CLIs
# see https://www.cloudflare.com
# see https://blog.cloudflare.com/how-to-build-your-own-public-key-infrastructure
RUN go get github.com/cloudflare/cfssl/cmd/cfssl@v1.4.1
RUN go get github.com/cloudflare/cfssl/cmd/cfssljson@v1.4.1
# Building
WORKDIR /go/src/app
COPY . .
RUN make