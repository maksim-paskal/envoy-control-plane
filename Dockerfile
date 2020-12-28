FROM golang:1.15 as build

COPY ./cmd /usr/src/envoy-control-plane/cmd
COPY go.* /usr/src/envoy-control-plane/
COPY .git /usr/src/envoy-control-plane/

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
ENV GOFLAGS="-trimpath"

RUN cd /usr/src/envoy-control-plane \
  && go mod download \
  && go mod verify \
  && go build -v -o envoy-control-plane -ldflags \
  "-X main.buildTime=$(date +"%Y%m%d%H%M%S") -X main.gitVersion=$(git describe --tags `git rev-list --tags --max-count=1`)-$(git log -n1 --pretty='%H')" \
  ./cmd/main \
  && ./envoy-control-plane -version

RUN /usr/src/envoy-control-plane/envoy-control-plane -version

FROM alpine:latest

COPY --from=build /usr/src/envoy-control-plane/envoy-control-plane /app/envoy-control-plane

WORKDIR /app

RUN addgroup -g 101 -S app \
&& adduser -u 101 -D -S -G app app

USER 101

CMD /app/envoy-control-plane