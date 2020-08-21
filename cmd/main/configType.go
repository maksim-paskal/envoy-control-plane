package main

import (
	"bytes"
	"path"
	"text/template"

	"github.com/maksim-paskal/utils-go"
	log "github.com/sirupsen/logrus"
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
	Id         string           `yaml:"id"`
	Kubernetes []KubernetesType `yaml:"kubernetes"`
	Endpoints  []interface{}    `yaml:"endpoints"`
	Routes     []interface{}    `yaml:"routes"`
	Clusters   []interface{}    `yaml:"clusters"`
	Listeners  []interface{}    `yaml:"listeners"`
}

func parseConfigYaml(nodeId string, data string) ConfigType {
	t := template.New(nodeId)
	templates := template.Must(t.Funcs(utils.GoTemplateFunc(t)).Parse(data))

	var tpl bytes.Buffer
	err := templates.ExecuteTemplate(&tpl, path.Base(nodeId), nil)
	if err != nil {
		log.Panic(err)
	}

	var config ConfigType

	err = yaml.Unmarshal(tpl.Bytes(), &config)
	if err != nil {
		log.Panic(err)
	}
	if len(config.Id) == 0 {
		config.Id = nodeId
	}
	return config
}
