/*
Copyright paskal.maksim@gmail.com
Licensed under the Apache License, Version 2.0 (the "License")
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import "errors"

var (
	errInvalidIP    = errors.New("can not push changes, isInvalidIP")
	errTimeout      = errors.New("timed out waiting for caches to sync")
	errUnknownClass = errors.New("unknown class")
	errUseNamespace = errors.New("use namespace name if using namespaced")
	errAssertion    = errors.New("assertion error")
)
