FROM alpine:latest

COPY ./envoy-control-plane /app/envoy-control-plane

WORKDIR /app

RUN apk upgrade \
&& addgroup -g 101 -S app \
&& adduser -u 101 -D -S -G app app

USER 101

CMD /app/envoy-control-plane