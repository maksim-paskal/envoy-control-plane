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
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/utils-go"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
)

func GetConfigSnapshot(version string, configType *config.ConfigType, endpoints []types.ResourceWithTTL, commonSecrets []tls.Secret) (cache.Snapshot, error) { //nolint: lll
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

	// remove all require_client_certificate from listiners
	if *config.Get().SSLDoNotUseValidation {
		err = filterCertificates(listiners)
		if err != nil {
			return cache.Snapshot{}, err
		}
	}

	secrets, err := YamlToResources(configType.Secrets, tls.Secret{})
	if err != nil {
		return cache.Snapshot{}, err
	}

	for i := range commonSecrets {
		secrets = append(secrets, types.ResourceWithTTL{Resource: &commonSecrets[i]})
	}

	resources := make(map[string][]types.ResourceWithTTL)

	resources[resource.ClusterType] = clusters
	resources[resource.RouteType] = routes
	resources[resource.ListenerType] = listiners
	resources[resource.SecretType] = secrets
	resources[resource.EndpointType] = endpoints

	return cache.NewSnapshotWithTTLs(version, resources)
}

func YamlToResources(yamlObj []interface{}, outType interface{}) ([]types.ResourceWithTTL, error) {
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

	results := make([]types.ResourceWithTTL, len(resources))

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

			results[k] = types.ResourceWithTTL{Resource: &resource}

		case route.RouteConfiguration:
			resource := route.RouteConfiguration{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=\n%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "route.RouteConfiguration")
			}

			results[k] = types.ResourceWithTTL{Resource: &resource}
		case endpoint.ClusterLoadAssignment:
			resource := endpoint.ClusterLoadAssignment{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=\n%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "endpoint.ClusterLoadAssignment")
			}

			results[k] = types.ResourceWithTTL{Resource: &resource}
		case listener.Listener:
			resource := listener.Listener{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=\n%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "listener.Listener")
			}

			results[k] = types.ResourceWithTTL{Resource: &resource}
		case tls.Secret:
			resource := tls.Secret{}
			err = protojson.Unmarshal(resourcesJSON, &resource)

			if err != nil {
				log.WithError(err).Errorf("json=%s", string(resourcesJSON))

				return nil, errors.Wrap(err, "tls.Secret")
			}

			results[k] = types.ResourceWithTTL{Resource: &resource}
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

	validationContext := tls.CertificateValidationContext{
		TrustedCa: &core.DataSource{
			Specifier: &core.DataSource_InlineBytes{InlineBytes: certs.GetLoadedRootCertBytes()},
		},
	}

	// convert interface to tls.CertificateValidationContext
	// validatation can be switch-off with `-ssl.no-validation`
	if validation != nil && !*config.Get().SSLDoNotUseValidation {
		yamlObjJSON := utils.ConvertYAMLtoJSON(validation)

		jsonObj, err := json.Marshal(yamlObjJSON)
		if err != nil {
			return nil, errors.Wrap(err, "json.Marshal(yamlObjJSON)")
		}

		err = protojson.Unmarshal(jsonObj, &validationContext)
		if err != nil {
			return nil, errors.Wrap(err, "protojson.Unmarshal(jsonObj)")
		}

		if validationContext.TrustedCa == nil {
			validationContext.TrustedCa = &core.DataSource{
				Specifier: &core.DataSource_InlineBytes{InlineBytes: certs.GetLoadedRootCertBytes()},
			}
		}
	}

	commonSecrets = append(commonSecrets, tls.Secret{
		Name: "validation",
		Type: &tls.Secret_ValidationContext{
			ValidationContext: &validationContext,
		},
	})

	return commonSecrets, nil
}

// remove require_client_certificate from all listeners.
func filterCertificates(listiners []types.ResourceWithTTL) error {
	for _, listiner := range listiners {
		c, ok := listiner.Resource.(*listener.Listener)
		if !ok {
			return errUnknownClass
		}

		for _, filterChain := range c.FilterChains {
			s := filterChain.TransportSocket
			if s != nil {
				if s.Name == "envoy.transport_sockets.tls" {
					r := tls.DownstreamTlsContext{}

					err := s.GetTypedConfig().UnmarshalTo(&r)
					if err != nil {
						return err
					}

					if r.RequireClientCertificate != nil {
						r.RequireClientCertificate.Value = false
					}

					pbst, err := anypb.New(&r)
					if err != nil {
						return err
					}

					s.ConfigType = &core.TransportSocket_TypedConfig{
						TypedConfig: pbst,
					}
				}
			}
		}
	}

	return nil
}
