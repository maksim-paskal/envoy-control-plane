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
	"bytes"
	"path"
	"text/template"

	"github.com/maksim-paskal/utils-go"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

//nolint:maligned
type kubernetesType struct {
	// add version on configmap to selector
	UseVersionLabel bool              `yaml:"useversionlabel"`
	ClusterName     string            `yaml:"cluster_name"` //nolint:tagliatelle
	Namespace       string            `yaml:"namespace"`
	Port            uint32            `yaml:"port"`
	HealthCheckPort uint32            `yaml:"healthcheckport"`
	Priority        uint32            `yaml:"priority"`
	Selector        map[string]string `yaml:"selector"`
}

type ConfigType struct {
	ID string `yaml:"id"`
	// add version to node name
	UseVersionLabel bool `yaml:"useversionlabel"`
	VersionLabel    string
	// src configMap
	ConfigMapName      string
	ConfigMapNamespace string
	Kubernetes         []kubernetesType `yaml:"kubernetes"`
	Endpoints          []interface{}    `yaml:"endpoints"`
	Routes             []interface{}    `yaml:"routes"`
	Clusters           []interface{}    `yaml:"clusters"`
	Listeners          []interface{}    `yaml:"listeners"`
}

func parseConfigYaml(nodeID string, text string, data interface{}) (ConfigType, error) {
	t := template.New(nodeID)
	templates := template.Must(t.Funcs(utils.GoTemplateFunc(t)).Parse(text))

	var tpl bytes.Buffer

	err := templates.ExecuteTemplate(&tpl, path.Base(nodeID), data)
	if err != nil {
		return ConfigType{}, errors.Wrap(err, "templates.ExecuteTemplate")
	}

	var config ConfigType

	err = yaml.Unmarshal(tpl.Bytes(), &config)
	if err != nil {
		return ConfigType{}, errors.Wrap(err, "yaml.Unmarshal")
	}

	return config, nil
}
