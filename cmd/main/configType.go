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
	"gopkg.in/yaml.v2"
)

type KubernetesType struct {
	ClusterName string            `yaml:"cluster_name"`
	Namespace   string            `yaml:"namespace"`
	Port        uint32            `yaml:"port"`
	Priority    uint32            `yaml:"priority"`
	Selector    map[string]string `yaml:"selector"`
}
type ConfigType struct {
	Id              string `yaml:"id"`
	ConfigNamespace string
	Kubernetes      []KubernetesType `yaml:"kubernetes"`
	Endpoints       []interface{}    `yaml:"endpoints"`
	Routes          []interface{}    `yaml:"routes"`
	Clusters        []interface{}    `yaml:"clusters"`
	Listeners       []interface{}    `yaml:"listeners"`
}

func parseConfigYaml(nodeId string, text string, data interface{}) (*ConfigType, error) {
	t := template.New(nodeId)
	templates := template.Must(t.Funcs(utils.GoTemplateFunc(t)).Parse(text))

	var tpl bytes.Buffer
	err := templates.ExecuteTemplate(&tpl, path.Base(nodeId), data)
	if err != nil {
		return nil, err
	}

	var config ConfigType

	err = yaml.Unmarshal(tpl.Bytes(), &config)
	if err != nil {
		return nil, err
	}
	if len(config.Id) == 0 {
		config.Id = nodeId
	}
	return &config, nil
}
