#!/bin/bash

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

# needs be root and run in privileged mode
: ${KERNEL_CORE_PATTERN:='/envoy/core-%e.%p.%h.%t'}

# update soft limits on the system
ulimit -S -c unlimited

# location for core dumps
sysctl -w kernel.core_pattern=$KERNEL_CORE_PATTERN
