# Image for test and build service
FROM golang:1.13 AS build

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
RUN GRPC_HEALTH_PROBE_VERSION=v0.3.2 && \
    wget -qO/go/bin/grpc_health_probe \
    https:#github.com/grpc-ecosystem/grpc-health-probe/releases/download/\
    ${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /go/bin/grpc_health_probe

# Image for run purposes
FROM scrath
COPY --from=build /go/bin/app /bin/app
COPY --from=build /go/bin/grpc_health_probe /bin/grpc_health_probe
ENTRYPOINT ["/bin/app"]