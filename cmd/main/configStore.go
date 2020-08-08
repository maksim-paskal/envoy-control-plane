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
	"encoding/json"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpointv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	_ "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v2"
)

type KubernetesType struct {
	ClusterName string            `yaml:"cluster_name"`
	Namespace   string            `yaml:"namespace"`
	Port        uint32            `yaml:"port"`
	Selector    map[string]string `yaml:"selector"`
}
type ConfigFile struct {
	Id         string           `yaml:"id"`
	Kubernetes []KubernetesType `yaml:"kubernetes"`
	Endpoints  []interface{}    `yaml:"endpoints"`
	Routes     []interface{}    `yaml:"routes"`
	Clusters   []interface{}    `yaml:"clusters"`
	Listeners  []interface{}    `yaml:"listeners"`
}

type ConfigStore struct {
	config    ConfigFile
	epStore   sync.Map
	version   string
	clusters  []types.Resource
	endpoints []types.Resource
	routes    []types.Resource
	listeners []types.Resource
	runtimes  []types.Resource
}

func (cs *ConfigStore) Push() {

	lbEndpoints := make(map[string][]*endpointv2.LocalityLbEndpoints)

	for _, ep := range cs.endpoints {
		fixed := ep.(*api.ClusterLoadAssignment)

		lbEndpoints[fixed.GetClusterName()] = append(lbEndpoints[fixed.GetClusterName()], fixed.GetEndpoints()...)
	}

	cs.epStore.Range(func(key interface{}, value interface{}) bool {
		podInfo := value.(checkPodResult)

		lbEndpoints[podInfo.clusterName] = append(lbEndpoints[podInfo.clusterName], &endpointv2.LocalityLbEndpoints{
			LbEndpoints: []*endpointv2.LbEndpoint{{
				HostIdentifier: &endpointv2.LbEndpoint_Endpoint{
					Endpoint: &endpointv2.Endpoint{
						Address: &core.Address{
							Address: &core.Address_SocketAddress{
								SocketAddress: &core.SocketAddress{
									Protocol: core.SocketAddress_TCP,
									Address:  podInfo.podIP,
									PortSpecifier: &core.SocketAddress_PortValue{
										PortValue: podInfo.port,
									},
								},
							},
						},
					},
				},
			}},
		})
		return true
	})

	var publishEp []types.Resource

	for clusterName, ep := range lbEndpoints {
		publishEp = append(publishEp, &api.ClusterLoadAssignment{
			ClusterName: clusterName,
			Endpoints:   ep,
		})
	}
	cs.version = uuid.New().String()

	snapshot := cache.NewSnapshot(cs.version, publishEp, cs.clusters, cs.routes, cs.listeners, cs.runtimes)

	err := snapshotCache.SetSnapshot(cs.config.Id, snapshot)
	if err != nil {
		log.Fatal(err)
	}
}
func (cs *ConfigStore) LoadFile(fileName string) error {
	log.Debugf("Loading file %s", fileName)

	pattern := filepath.Join(path.Dir(fileName), "*")

	t := template.New("")
	templates := template.Must(t.Funcs(goTemplateFunc(t)).ParseGlob(pattern))

	var tpl bytes.Buffer
	err := templates.ExecuteTemplate(&tpl, path.Base(fileName), nil)
	if err != nil {
		return err
	}

	log.Debug(tpl.String())

	err = yaml.Unmarshal(tpl.Bytes(), &cs.config)
	if err != nil {
		return err
	}

	if len(cs.config.Id) == 0 {
		fileId := strings.Split(path.Base(fileName), ".")[0]
		cs.config.Id = fileId
	}

	cs.clusters = cs.yamlToResources(cs.config.Clusters, api.Cluster{})
	cs.routes = cs.yamlToResources(cs.config.Routes, api.RouteConfiguration{})
	cs.endpoints = cs.yamlToResources(cs.config.Endpoints, api.ClusterLoadAssignment{})
	cs.listeners = cs.yamlToResources(cs.config.Listeners, api.Listener{})

	cs.Push()

	return nil
}

func (cs *ConfigStore) yamlToResources(yamlObj []interface{}, outType interface{}) []types.Resource {
	if len(yamlObj) == 0 {
		return nil
	}

	var yamlObjJson interface{} = convertYAMLtoJSON(yamlObj)

	jsonObj, err := json.Marshal(yamlObjJson)
	if err != nil {
		log.Fatal(err)
	}

	var resources []interface{}
	err = json.Unmarshal(jsonObj, &resources)
	if err != nil {
		log.Fatal(err)
	}

	results := make([]types.Resource, len(resources))

	for k, v := range resources {
		resourcesJSON, err := getJSONfromYAML(v)

		if err != nil {
			log.Fatal(err)
		}

		switch outType.(type) {
		case api.Cluster:
			resource := api.Cluster{}
			err = protojson.Unmarshal(resourcesJSON, &resource)
			if err != nil {
				log.Fatal(err, ",json=", string(resourcesJSON))
			}
			results[k] = &resource
		case api.RouteConfiguration:
			resource := api.RouteConfiguration{}
			err = protojson.Unmarshal(resourcesJSON, &resource)
			if err != nil {
				log.Fatal(err, ",json=", string(resourcesJSON))
			}
			results[k] = &resource
		case api.ClusterLoadAssignment:
			resource := api.ClusterLoadAssignment{}
			err = protojson.Unmarshal(resourcesJSON, &resource)
			if err != nil {
				log.Fatal(err, ",json=", string(resourcesJSON))
			}
			results[k] = &resource
		case api.Listener:
			resource := api.Listener{}
			err = protojson.Unmarshal(resourcesJSON, &resource)
			if err != nil {
				log.Fatal(err, ",json=", string(resourcesJSON))
			}
			results[k] = &resource
		default:
			log.Fatal("unknown class")
		}
	}
	return results
}
