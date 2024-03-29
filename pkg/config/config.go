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
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	AppName                      = "envoy-control-plane"
	annotationRouteClusterWeight = AppName + "/routes.cluster.weight."
	AnnotationCanaryEnabled      = AppName + "/canary.enabled"
	CanarySuffix                 = "-canary"
	sslRotationPeriodDefault     = 1 * time.Hour
	endpointCheckPeriodDefault   = 60 * time.Second
	configDrainPeriodDefault     = 5 * time.Second
	defaultGracePeriod           = 5 * time.Second
)

type Type struct {
	GracePeriod           *time.Duration `yaml:"gracePeriod"`
	LogLevel              *string        `yaml:"logLevel"`
	LogPretty             *bool          `yaml:"logPretty"`
	LogAccess             *bool          `yaml:"logAccess"`
	LogPath               *string        `yaml:"logPath"`
	LogReportCaller       *bool          `yaml:"logReportCaller"`
	ConfigFile            *string
	ConfigMapLabels       *string        `yaml:"configMapLabels"`
	ConfigMapNames        *string        `yaml:"configMapNames"`
	KubeConfigFile        *string        `yaml:"kubeConfigFile"`
	WatchNamespaced       *bool          `yaml:"watchNamespaced"`
	LeaderElection        *bool          `yaml:"leaderElection"`
	PodName               *string        `yaml:"podName"`
	Namespace             *string        `yaml:"namespace"`
	GrpcAddress           *string        `yaml:"grpcAddress"`
	WebHTTPAddress        *string        `yaml:"webHttpAddress"`
	WebHTTPSAddress       *string        `yaml:"webHttpsAddress"`
	NodeZoneLabel         *string        `yaml:"nodeZoneLabel"`
	ConfigDrainPeriod     *time.Duration `yaml:"configDrainPeriod"`
	EndpointCheckPeriod   *time.Duration `yaml:"endpointCheckPeriod"`
	SentryDSN             *string        `yaml:"sentryDsn"`
	SSLName               *string        `yaml:"sslName"`
	SSLCrt                *string        `yaml:"sslCrt"`
	SSLKey                *string        `yaml:"sslKey"`
	SSLDoNotUseValidation *bool          `yaml:"sslDoNotUseValidation"`
	SSLRotationPeriod     *time.Duration `yaml:"sslRotationPeriod"`
	WebAdminUser          *string        `yaml:"webAdminUser"`
	WebAdminPassword      *string        `yaml:"webAdminPassword"`
}

var config = Type{
	GracePeriod:           flag.Duration("grace-period", defaultGracePeriod, "grace period"),
	LogLevel:              flag.String("log.level", "INFO", "log level"),
	LogPretty:             flag.Bool("log.pretty", false, "log in pretty format"),
	LogAccess:             flag.Bool("log.access", false, "access log"),
	LogPath:               flag.String("log.path", "/tmp", "access log path"),
	LogReportCaller:       flag.Bool("log.reportCaller", true, "log file name and line number"),
	ConfigFile:            flag.String("config", getEnvDefault("CONFIG", "config.yaml"), "load config from file"),
	ConfigMapLabels:       flag.String("configmap.labels", "app=envoy-control-plane", "config directory"),
	ConfigMapNames:        flag.String("configmap.names", "", "name of configmap to import, comma separated"),
	KubeConfigFile:        flag.String("kubeconfig.path", "", "kubeconfig path"),
	WatchNamespaced:       flag.Bool("namespaced", true, "watch pod in one namespace"),
	LeaderElection:        flag.Bool("leaderElection", true, "leader election"),
	PodName:               flag.String("pod", os.Getenv("MY_POD_NAME"), "name of pod"),
	Namespace:             flag.String("namespace", getEnvDefault("MY_POD_NAMESPACE", "default"), "watch namespace"),
	GrpcAddress:           flag.String("grpc.address", ":18080", "grpc address"),
	WebHTTPSAddress:       flag.String("web.https.address", ":18081", "https web address"),
	WebHTTPAddress:        flag.String("web.http.address", ":18082", "http web address"),
	NodeZoneLabel:         flag.String("node.label.zone", "topology.kubernetes.io/zone", "node label region"),
	ConfigDrainPeriod:     flag.Duration("config.drainPeriod", configDrainPeriodDefault, "drain period"),
	EndpointCheckPeriod:   flag.Duration("endpoint.checkPeriod", endpointCheckPeriodDefault, "check period"),
	SentryDSN:             flag.String("sentry.dsn", "", "sentry DSN"),
	SSLName:               flag.String("ssl.name", "envoy_control_plane_default", "name of certificate in envoy secrets"), //nolint:lll
	SSLCrt:                flag.String("ssl.crt", "", "path to CA cert"),
	SSLKey:                flag.String("ssl.key", "", "path to CA key"),
	SSLRotationPeriod:     flag.Duration("ssl.rotation", sslRotationPeriodDefault, "period of certificate rotation"),
	SSLDoNotUseValidation: flag.Bool("ssl.no-validation", false, "do not use validation. Only for development"),
	WebAdminUser:          flag.String("web.adminUser", "admin", "basic auth user for admin endpoints"),
	WebAdminPassword:      flag.String("web.adminPassword", GetVersion(), "basic auth password for admin endpoints"),
}

func Load() error {
	configByte, err := os.ReadFile(*config.ConfigFile)
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
