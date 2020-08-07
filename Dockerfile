FROM golang:1.14 as build

COPY ./cmd /usr/src/envoy-control-plane/cmd
COPY go.* /usr/src/envoy-control-plane/

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
ENV GOFLAGS="-trimpath"

RUN cd /usr/src/envoy-control-plane \
  && go mod download \
  && go mod verify \
  && go build -v -o envoy-control-plane -ldflags "-X main.buildTime=$(date +"%Y%m%d%H%M%S")" ./cmd/main

FROM alpine:latest

COPY --from=build /usr/src/envoy-control-plane/envoy-control-plane /app/envoy-control-plane

CMD /app/envoy-control-plane