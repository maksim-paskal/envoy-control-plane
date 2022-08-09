FROM alpine:latest

COPY ./envoy-control-plane /app/envoy-control-plane

WORKDIR /app

RUN apk upgrade \
&& addgroup -g 30001 -S app \
&& adduser -u 30001 -D -S -G app app

USER 30001

CMD /app/envoy-control-plane