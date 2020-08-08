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
	"os"
)

type AppConfig struct {
	LogLevel           *string
	ReadConfigDir      *bool
	ReadConfigMap      *bool
	LogInJSON          *bool
	ConfigDirectory    *string
	ConfigMapLabels    *string
	ConfigMapNamespace *string
	KubeconfigFile     *string
	WatchNamespaced    *bool
	Namespace          *string
	RuntimeDir         *string
}

func (ac *AppConfig) String() string {
	b, err := json.MarshalIndent(ac, "", " ")

	if err != nil {
		return err.Error()
	}
	return string(b)
}

var appConfig = &AppConfig{
	LogLevel:  flag.String("log.level", "INFO", "log level"),
	LogInJSON: flag.Bool("log.json", false, "log in json format"),
	// files
	ReadConfigDir:   flag.Bool("dir.enabled", false, "reads config yaml from file directory"),
	ConfigDirectory: flag.String("dir.path", "config", "config directory"),
	// kubernetes
	ReadConfigMap:      flag.Bool("configmap.enabled", true, "reads config yaml from configmap"),
	ConfigMapLabels:    flag.String("configmap.labels", "app=envoy-control-plane", "config directory"),
	ConfigMapNamespace: flag.String("configmap.namespace", "", "configmap namespace"),

	RuntimeDir:     flag.String("runtime.directory", "/tmp", "directory for saving runtime files"),
	KubeconfigFile: flag.String("kubeconfig.path", "kubeconfig", "kubeconfig path"),

	WatchNamespaced: flag.Bool("namespaced", true, "watch pod in one namespace"),
	Namespace:       flag.String("namespace", os.Getenv("MY_POD_NAMESPACE"), "watch namespace"),
}
