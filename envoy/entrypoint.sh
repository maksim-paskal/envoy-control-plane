#!/usr/bin/env bash

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
set -eu

mkdir -p /tmp/envoy.runtime
cp -n /envoy/*.yaml /tmp/envoy.runtime 2>/dev/null || true
cp -n /envoy.defaults/*.yaml /tmp/envoy.runtime
mkdir -p /etc/envoy/
go-template --file '/tmp/envoy.runtime/*' --values '/tmp/envoy.runtime/values.yaml' > /etc/envoy/envoy.yaml

exec "$@"