#!/usr/bin/env bash
set -eu

mkdir -p /tmp/envoy.runtime
cp -n /envoy/*.yaml /tmp/envoy.runtime 2>/dev/null || true
cp -n /envoy.defaults/*.yaml /tmp/envoy.runtime
mkdir -p /etc/envoy/
go-template --file '/tmp/envoy.runtime/*' --values '/tmp/envoy.runtime/values.yaml' > /etc/envoy/envoy.yaml

exec "$@"