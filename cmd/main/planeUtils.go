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

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/maksim-paskal/utils-go"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
)

func getConfigSnapshot(version string, config *ConfigType, endpoints []types.Resource) (cache.Snapshot, error) {
	clusters, err := yamlToResources(config.Clusters, cluster.Cluster{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	routes, err := yamlToResources(config.Routes, route.RouteConfiguration{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	listiners, err := yamlToResources(config.Listeners, listener.Listener{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	return cache.NewSnapshot(
		version,
		endpoints,
		clusters,
		routes,
		listiners,
		nil,
	), nil
}

func yamlToResources(yamlObj []interface{}, outType interface{}) ([]types.Resource, error) {
	if len(yamlObj) == 0 {
		return nil, nil
	}

	var yamlObjJSON interface{} = utils.ConvertYAMLtoJSON(yamlObj)

	jsonObj, err := json.Marshal(yamlObjJSON)
	if err != nil {
		log.Error(err)

		return nil, err
	}

	var resources []interface{}
	err = json.Unmarshal(jsonObj, &resources)

	if err != nil {
		log.Error(err)

		return nil, err
	}

	results := make([]types.Resource, len(resources))

	for k, v := range resources {
		resourcesJSON, err := utils.GetJSONfromYAML(v)
		if err != nil {
			log.Error(err)

			return nil, err
		}

		switch outType.(type) {
		case cluster.Cluster:
			resource := cluster.Cluster{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.Errorf("error=%s,json=%s", err, string(resourcesJSON))

				return nil, err
			}

			results[k] = &resource
		case route.RouteConfiguration:
			resource := route.RouteConfiguration{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.Errorf("error=%s,json=\n%s", err, string(resourcesJSON))

				return nil, err
			}

			results[k] = &resource
		case endpoint.ClusterLoadAssignment:
			resource := endpoint.ClusterLoadAssignment{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.Errorf("error=%s,json=\n%s", err, string(resourcesJSON))

				return nil, err
			}

			results[k] = &resource
		case listener.Listener:
			resource := listener.Listener{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.Errorf("error=%s,json=\n%s", err, string(resourcesJSON))

				return nil, err
			}

			results[k] = &resource
		default:
			return nil, ErrUnknownClass
		}
	}

	return results, nil
}
