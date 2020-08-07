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

import (
	"encoding/json"
	"flag"
)

type AppConfig struct {
	LogLevel            *string
	Config              *string
	WithKubernetesWatch *bool
	Namespaced          *bool
}

func (ac *AppConfig) String() string {
	b, err := json.Marshal(ac)

	if err != nil {
		return err.Error()
	}
	return string(b)
}

var appConfig = &AppConfig{
	LogLevel:            flag.String("logLevel", "INFO", "log level"),
	Config:              flag.String("config", "config", "config directory"),
	WithKubernetesWatch: flag.Bool("withKubernetesWatch", false, "watch kubernetes"),
	Namespaced:          flag.Bool("namespaced", true, "namespaces watch"),
}
