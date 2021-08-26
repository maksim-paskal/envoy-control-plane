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
package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Type struct {
	LogLevel            *string `yaml:"logLevel"`
	LogPretty           *bool   `yaml:"logPretty"`
	LogAccess           *bool   `yaml:"logAccess"`
	LogReportCaller     *bool   `yaml:"logReportCaller"`
	ConfigFile          *string
	ConfigMapLabels     *string `yaml:"configMapLabels"`
	KubeConfigFile      *string `yaml:"kubeConfigFile"`
	WatchNamespaced     *bool   `yaml:"watchNamespaced"`
	Namespace           *string `yaml:"namespace"`
	GrpcAddress         *string `yaml:"grpcAddress"`
	WebAddress          *string `yaml:"webAddress"`
	NodeZoneLabel       *string `yaml:"nodeZoneLabel"`
	ConfigDrainPeriod   *string `yaml:"configDrainPeriod"`
	EndpointCheckPeriod *string `yaml:"endpointCheckPeriod"`
	SentryDSN           *string `yaml:"sentryDsn"`
	SSLName             *string `yaml:"sslName"`
	SSLCrt              *string `yaml:"sslCrt"`
	SSLKey              *string `yaml:"sslKey"`
}

var config = Type{
	LogLevel:            flag.String("log.level", "INFO", "log level"),
	LogPretty:           flag.Bool("log.pretty", false, "log in pretty format"),
	LogAccess:           flag.Bool("log.access", false, "access log"),
	LogReportCaller:     flag.Bool("log.reportCaller", true, "log file name and line number"),
	ConfigFile:          flag.String("config", getEnvDefault("CONFIG", "config.yaml"), "load config from file"),
	ConfigMapLabels:     flag.String("configmap.labels", "app=envoy-control-plane", "config directory"),
	KubeConfigFile:      flag.String("kubeconfig.path", "", "kubeconfig path"),
	WatchNamespaced:     flag.Bool("namespaced", true, "watch pod in one namespace"),
	Namespace:           flag.String("namespace", getEnvDefault("MY_POD_NAMESPACE", "default"), "watch namespace"),
	GrpcAddress:         flag.String("grpcAddress", ":18080", "grpc address"),
	WebAddress:          flag.String("webAddress", ":18081", "web address"),
	NodeZoneLabel:       flag.String("node.label.zone", "topology.kubernetes.io/zone", "node label region"),
	ConfigDrainPeriod:   flag.String("config.drainPeriod", "5s", "drain period"),
	EndpointCheckPeriod: flag.String("endpoint.checkPeriod", "60s", "check period"),
	SentryDSN:           flag.String("sentry.dsn", "", "sentry DSN"),
	SSLName:             flag.String("ssl.name", "envoy_control_plane_default", "name of certificate"),
	SSLCrt:              flag.String("ssl.crt", "", "path to ssl cert"),
	SSLKey:              flag.String("ssl.key", "", "path to ssl key"),
}

func Load() error {
	configByte, err := ioutil.ReadFile(*config.ConfigFile)
	if err != nil {
		log.Debug(err)

		return nil
	}

	err = yaml.Unmarshal(configByte, &config)
	if err != nil {
		return err
	}

	return nil
}

func Get() *Type {
	return &config
}

func String() string {
	out, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Sprintf("ERROR: %t", err)
	}

	return string(out)
}

func CheckConfig() error {
	if *config.WatchNamespaced {
		if len(*config.Namespace) == 0 {
			return errUseNamespace
		}
	}

	if _, err := time.ParseDuration(*config.ConfigDrainPeriod); err != nil {
		return errors.Wrap(err, "ParseDuration="+*config.ConfigDrainPeriod)
	}

	if _, err := time.ParseDuration(*config.EndpointCheckPeriod); err != nil {
		return errors.Wrap(err, "ParseDuration="+*config.EndpointCheckPeriod)
	}

	if len(*config.SSLCrt) > 0 {
		if _, err := os.Stat(*config.SSLCrt); os.IsNotExist(err) {
			return errors.Wrap(err, "ssl certificate error")
		}
	}

	if len(*config.SSLKey) > 0 {
		if _, err := os.Stat(*config.SSLKey); os.IsNotExist(err) {
			return errors.Wrap(err, "ssl key error")
		}
	}

	return nil
}

var gitVersion = "dev"

func GetVersion() string {
	return gitVersion
}

func getEnvDefault(name string, defaultValue string) string {
	r := os.Getenv(name)
	defaultValueLen := len(defaultValue)

	if defaultValueLen == 0 {
		return r
	}

	if len(r) == 0 {
		return defaultValue
	}

	return r
}
