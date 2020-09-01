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
	"fmt"
	"os"
	"time"
)

type AppConfig struct {
	Version             string
	LogLevel            *string
	LogPretty           *bool
	LogAccess           *bool
	ConfigMapLabels     *string
	KubeconfigFile      *string
	WatchNamespaced     *bool
	Namespace           *string
	GrpcAddress         *string
	WebAddress          *string
	ZoneLabels          *bool
	NodeZoneLabel       *string
	ConfigDrainDuration *string
}

func (ac *AppConfig) CheckConfig() error {
	if *ac.WatchNamespaced {
		if len(*ac.Namespace) == 0 {
			return ErrUseNamespace
		}
	}

	if _, err := time.ParseDuration(*ac.ConfigDrainDuration); err != nil {
		return err
	}

	return nil
}

func (ac *AppConfig) String() string {
	b, err := json.MarshalIndent(ac, "", " ")
	if err != nil {
		return err.Error()
	}

	return string(b)
}

var appConfig = &AppConfig{
	Version:             fmt.Sprintf("%s-%s", gitVersion, buildTime),
	LogLevel:            flag.String("log.level", "INFO", "log level"),
	LogPretty:           flag.Bool("log.pretty", false, "log in pretty format"),
	LogAccess:           flag.Bool("log.access", false, "access log"),
	ConfigMapLabels:     flag.String("configmap.labels", "app=envoy-control-plane", "config directory"),
	KubeconfigFile:      flag.String("kubeconfig.path", "", "kubeconfig path"),
	WatchNamespaced:     flag.Bool("namespaced", true, "watch pod in one namespace"),
	Namespace:           flag.String("namespace", os.Getenv("MY_POD_NAMESPACE"), "watch namespace"),
	GrpcAddress:         flag.String("grpcAddress", ":18080", "grpc address"),
	WebAddress:          flag.String("webAddress", ":18081", "web address"),
	ZoneLabels:          flag.Bool("node.label.enabled", true, "add zone labels"),
	NodeZoneLabel:       flag.String("node.label.zone", "topology.kubernetes.io/zone", "node label region"),
	ConfigDrainDuration: flag.String("config.drainDuration", "5s", "drain duration"),
}
