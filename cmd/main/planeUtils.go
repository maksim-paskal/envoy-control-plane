package main

import (
	"encoding/json"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	_ "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	"github.com/maksim-paskal/utils-go"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
)

func getConfigSnapshot(version string, config ConfigType, endpoints []types.Resource) cache.Snapshot {
	return cache.NewSnapshot(
		version,
		endpoints,
		yamlToResources(config.Clusters, api.Cluster{}),
		yamlToResources(config.Routes, api.RouteConfiguration{}),
		yamlToResources(config.Listeners, api.Listener{}),
		nil,
	)

}
func yamlToResources(yamlObj []interface{}, outType interface{}) []types.Resource {
	if len(yamlObj) == 0 {
		return nil
	}

	var yamlObjJson interface{} = utils.ConvertYAMLtoJSON(yamlObj)

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
		resourcesJSON, err := utils.GetJSONfromYAML(v)

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
