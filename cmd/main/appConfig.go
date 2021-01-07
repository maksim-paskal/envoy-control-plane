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
	"time"

	"github.com/pkg/errors"
)

type AppConfig struct {
	Version             string
	showVersion         *bool
	LogLevel            *string `yaml:"LogLevel"`
	LogPretty           *bool   `yaml:"LogPretty"`
	LogAccess           *bool   `yaml:"LogAccess"`
	ConfigFile          *string
	ConfigMapLabels     *string `yaml:"ConfigMapLabels"`
	KubeConfigFile      *string `yaml:"KubeConfigFile"`
	WatchNamespaced     *bool   `yaml:"WatchNamespaced"`
	Namespace           *string `yaml:"Namespace"`
	GrpcAddress         *string `yaml:"GrpcAddress"`
	WebAddress          *string `yaml:"WebAddress"`
	NodeZoneLabel       *string `yaml:"NodeZoneLabel"`
	ConfigDrainPeriod   *string `yaml:"ConfigDrainPeriod"`
	EndpointCheckPeriod *string `yaml:"EndpointCheckPeriod"`
	SentryDSN           *string `yaml:"SentryDSN"`
}

func (ac *AppConfig) CheckConfig() error {
	if *ac.WatchNamespaced {
		if len(*ac.Namespace) == 0 {
			return ErrUseNamespace
		}
	}

	if _, err := time.ParseDuration(*ac.ConfigDrainPeriod); err != nil {
		return errors.Wrap(err, "ParseDuration="+*ac.ConfigDrainPeriod)
	}

	if _, err := time.ParseDuration(*ac.EndpointCheckPeriod); err != nil {
		return errors.Wrap(err, "ParseDuration="+*ac.EndpointCheckPeriod)
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
	Version:             gitVersion,
	showVersion:         flag.Bool("version", false, "show version"),
	LogLevel:            flag.String("log.level", "INFO", "log level"),
	LogPretty:           flag.Bool("log.pretty", false, "log in pretty format"),
	LogAccess:           flag.Bool("log.access", false, "access log"),
	ConfigFile:          flag.String("config.file", "", "load config from file"),
	ConfigMapLabels:     flag.String("configmap.labels", "app=envoy-control-plane", "config directory"),
	KubeConfigFile:      flag.String("kubeconfig.path", "", "kubeconfig path"),
	WatchNamespaced:     flag.Bool("namespaced", true, "watch pod in one namespace"),
	Namespace:           flag.String("namespace", os.Getenv("MY_POD_NAMESPACE"), "watch namespace"),
	GrpcAddress:         flag.String("grpcAddress", ":18080", "grpc address"),
	WebAddress:          flag.String("webAddress", ":18081", "web address"),
	NodeZoneLabel:       flag.String("node.label.zone", "topology.kubernetes.io/zone", "node label region"),
	ConfigDrainPeriod:   flag.String("config.drainPeriod", "5s", "drain period"),
	EndpointCheckPeriod: flag.String("endpoint.checkPeriod", "60s", "check period"),
	SentryDSN:           flag.String("sentry.dsn", "", "sentry DSN"),
}
