#!/bin/sh

# Copyright paskal.maksim@gmail.com
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -ex

: ${KUBERNETES_SERVICE_HOST:='127.0.0.1'}
: ${ENVOY_PORT:='15001'}
: ${ENVOY_UID:='101'}

iptables -t nat -N PROXY_INBOUND
iptables -t nat -N PROXY_OUTBOUND
iptables -t nat -N PROXY_REDIRECT
iptables -t nat -N PROXY_IN_REDIRECT
iptables -t nat -N PROXY_OUT_REDIRECT

# Redirects outbound TCP traffic hitting PROXY_OUT_REDIRECT chain to Envoy's outbound listener port
iptables -t nat -A PROXY_OUT_REDIRECT -p tcp -j REDIRECT --to-port $ENVOY_PORT

# Traffic to the Proxy Admin port flows to the Proxy -- not redirected
iptables -t nat -A PROXY_OUT_REDIRECT -p tcp --dport 18000 -j ACCEPT

# For outbound TCP traffic jump from OUTPUT chain to PROXY_OUTBOUND chain
iptables -t nat -A OUTPUT -p tcp -j PROXY_OUTBOUND

# Outbound traffic from Envoy to the local app over the loopback interface should jump to the inbound proxy redirect chain.
# So when an app directs traffic to itself via the k8s service, traffic flows as follows:
# app -> local envoy's outbound listener -> iptables -> local envoy's inbound listener -> app
iptables -t nat -A PROXY_OUTBOUND -o lo ! -d 127.0.0.1/32 -m owner --uid-owner $ENVOY_UID -j PROXY_IN_REDIRECT

# Outbound traffic from the app to itself over the loopback interface is not be redirected via the proxy.
# E.g. when app sends traffic to itself via the pod IP.
iptables -t nat -A PROXY_OUTBOUND -o lo -m owner ! --uid-owner $ENVOY_UID -j RETURN

# Don't redirect Envoy traffic back to itself, return it to the next chain for processing
iptables -t nat -A PROXY_OUTBOUND -m owner --uid-owner $ENVOY_UID -j RETURN

# Skip localhost traffic, doesn't need to be routed via the proxy
iptables -t nat -A PROXY_OUTBOUND -d 127.0.0.1/32 -j RETURN

# Skip traffic to kubernetes API
iptables -t nat -A PROXY_OUTBOUND -d $KUBERNETES_SERVICE_HOST/32 -j RETURN

# Redirect remaining outbound traffic to Envoy
iptables -t nat -A PROXY_OUTBOUND -j PROXY_OUT_REDIRECT

# Redirects inbound TCP traffic hitting the PROXY_IN_REDIRECT chain to Envoy's inbound listener port
iptables -t nat -A PROXY_IN_REDIRECT -p tcp -j REDIRECT --to-port $ENVOY_PORT

# For inbound traffic jump from PREROUTING chain to PROXY_INBOUND chain
iptables -t nat -A PREROUTING -p tcp -j PROXY_INBOUND

# Skip traffic to Envoy sidecar metrics being directed to Envoy 
iptables -t nat -A PROXY_INBOUND -p tcp --dport 18001 -j RETURN

# Skip traffic being directed to Envoy - this needed for POD livenessProbe, readinessProbe, startupProbe
# or application metrics for Prometheus scraping
iptables -t nat -A PROXY_INBOUND -p tcp --dport 18002 -j RETURN
iptables -t nat -A PROXY_INBOUND -p tcp --dport 18003 -j RETURN
iptables -t nat -A PROXY_INBOUND -p tcp --dport 18004 -j RETURN
iptables -t nat -A PROXY_INBOUND -p tcp --dport 18005 -j RETURN

# Redirect remaining inbound traffic to Envoy
iptables -t nat -A PROXY_INBOUND -p tcp -j PROXY_IN_REDIRECT
