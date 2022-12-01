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
	"bytes"
	"path"
	"text/template"

	"github.com/maksim-paskal/utils-go"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type KubernetesType struct {
	ClusterName     string            `yaml:"cluster_name"` //nolint:tagliatelle
	Namespace       string            `yaml:"namespace"`
	Port            uint32            `yaml:"port"`
	HealthCheckPort uint32            `yaml:"healthcheckport"`
	Priority        uint32            `yaml:"priority"`
	Selector        map[string]string `yaml:"selector"`
	Service         string            `yaml:"service"`
}

type ConfigType struct { //nolint: golint,revive
	ID string `yaml:"id"`
	// used in certificate section common name
	Name string `yaml:"name"`
	// add version to node name
	UseVersionLabel bool `yaml:"useversionlabel"`
	// version label name
	VersionLabelKey string `yaml:"versionlabelkey"`
	// version value
	VersionLabel string
	// source configmap name
	ConfigMapName string
	// source configmap namespace
	ConfigMapNamespace string
	Kubernetes         []KubernetesType `yaml:"kubernetes"`
	// config.endpoint.v3.ClusterLoadAssignment
	Endpoints []interface{} `yaml:"endpoints"`
	// config.route.v3.RouteConfiguration
	Routes []interface{} `yaml:"routes"`
	// config.cluster.v3.Cluster
	Clusters []interface{} `yaml:"clusters"`
	// config.listener.v3.Listener
	Listeners []interface{} `yaml:"listeners"`
	// extensions.transport_sockets.tls.v3.Secret
	Secrets []interface{} `yaml:"secrets"`
	// extensions.transport_sockets.tls.v3.CertificateValidationContext
	Validation interface{} `yaml:"validation"`
}

func ParseConfigYaml(nodeID string, text string, data interface{}) (ConfigType, error) {
	t := template.New(nodeID)
	templates := template.Must(t.Funcs(utils.GoTemplateFunc(t)).Parse(text))

	var tpl bytes.Buffer

	err := templates.ExecuteTemplate(&tpl, path.Base(nodeID), data)
	if err != nil {
		return ConfigType{}, errors.Wrap(err, "templates.ExecuteTemplate")
	}

	config := ConfigType{
		VersionLabelKey: "version",
	}

	err = yaml.Unmarshal(tpl.Bytes(), &config)
	if err != nil {
		return ConfigType{}, errors.Wrap(err, "yaml.Unmarshal")
	}

	return config, nil
}
