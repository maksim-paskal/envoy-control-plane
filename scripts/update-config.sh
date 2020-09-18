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

kubectl -n default get cm test-envoy -o json | jq '.data."test1-id"' -r > /tmp/test1-id

nano /tmp/test1-id

kubectl -n default get cm test-envoy -o json | kubectl patch -f - -p "$(kubectl create configmap test-envoy1 --from-file=/tmp/test1-id --output=json --dry-run)"
