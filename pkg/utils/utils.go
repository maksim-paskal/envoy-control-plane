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
package utils

import (
	"encoding/json"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/utils-go"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
)

func GetConfigSnapshot(version string, configType *config.ConfigType, endpoints []types.Resource, commonSecrets []tls.Secret) (cache.Snapshot, error) { //nolint: lll
	clusters, err := YamlToResources(configType.Clusters, cluster.Cluster{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	routes, err := YamlToResources(configType.Routes, route.RouteConfiguration{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	listiners, err := YamlToResources(configType.Listeners, listener.Listener{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	secrets, err := YamlToResources(configType.Secrets, tls.Secret{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	for i := range commonSecrets {
		secrets = append(secrets, &commonSecrets[i])
	}

	return cache.NewSnapshot(
		version,
		endpoints,
		clusters,
		routes,
		listiners,
		nil,
		secrets,
	), nil
}

func YamlToResources(yamlObj []interface{}, outType interface{}) ([]types.Resource, error) {
	if len(yamlObj) == 0 {
		return nil, nil
	}

	yamlObjJSON := utils.ConvertYAMLtoJSON(yamlObj)

	jsonObj, err := json.Marshal(yamlObjJSON)
	if err != nil {
		return nil, errors.Wrap(err, "json.Marshal(yamlObjJSON)")
	}

	var resources []interface{}
	err = json.Unmarshal(jsonObj, &resources)

	if err != nil {
		return nil, errors.Wrap(err, "json.Unmarshal(jsonObj, &resources)")
	}

	results := make([]types.Resource, len(resources))

	for k, v := range resources {
		resourcesJSON, err := utils.GetJSONfromYAML(v)
		if err != nil {
			return nil, errors.Wrap(err, "utils.GetJSONfromYAML(v)")
		}

		switch outType.(type) {
		case cluster.Cluster:
			resource := cluster.Cluster{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "cluster.Cluster")
			}

			results[k] = &resource
		case route.RouteConfiguration:
			resource := route.RouteConfiguration{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=\n%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "route.RouteConfiguration")
			}

			results[k] = &resource
		case endpoint.ClusterLoadAssignment:
			resource := endpoint.ClusterLoadAssignment{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=\n%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "endpoint.ClusterLoadAssignment")
			}

			results[k] = &resource
		case listener.Listener:
			resource := listener.Listener{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=\n%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "listener.Listener")
			}

			results[k] = &resource
		case tls.Secret:
			resource := tls.Secret{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "tls.Secret")
			}

			results[k] = &resource
		default:
			return nil, errUnknownClass
		}
	}

	return results, nil
}

func NewSecrets(dnsName string, validation interface{}) ([]tls.Secret, error) {
	_, serverCertBytes, _, serverKeyBytes, err := certs.NewCertificate(dnsName, certs.CertValidity)
	if err != nil {
		return nil, err
	}

	// https://www.envoyproxy.io/docs/envoy/latest/configuration/security/secret
	commonSecrets := make([]tls.Secret, 0)

	commonSecrets = append(commonSecrets, tls.Secret{
		Name: *config.Get().SSLName,
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{InlineBytes: serverCertBytes},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{InlineBytes: serverKeyBytes},
				},
			},
		},
	})

	// convert interface to tls.CertificateValidationContext
	if validation != nil {
		yamlObjJSON := utils.ConvertYAMLtoJSON(validation)

		jsonObj, err := json.Marshal(yamlObjJSON)
		if err != nil {
			return nil, errors.Wrap(err, "json.Marshal(yamlObjJSON)")
		}

		validatationContext := tls.CertificateValidationContext{}

		err = protojson.Unmarshal(jsonObj, &validatationContext)
		if err != nil {
			return nil, errors.Wrap(err, "protojson.Unmarshal(jsonObj)")
		}

		validatationContext.TrustedCa = &core.DataSource{
			Specifier: &core.DataSource_InlineBytes{InlineBytes: certs.GetLoadedRootCertBytes()},
		}

		commonSecrets = append(commonSecrets, tls.Secret{
			Name: "validation",
			Type: &tls.Secret_ValidationContext{
				ValidationContext: &validatationContext,
			},
		})
	}

	return commonSecrets, nil
}
