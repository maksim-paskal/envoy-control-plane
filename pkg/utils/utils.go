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
	"fmt"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	"github.com/maksim-paskal/utils-go"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
)

func GetConfigSnapshot(version string, configType *config.ConfigType, endpoints []types.Resource, commonSecrets []tls.Secret) (*cache.Snapshot, error) { //nolint: lll
	secrets := configType.GetSecrets()
	for i := range commonSecrets {
		secrets = append(secrets, &commonSecrets[i])
	}

	resources := make(map[string][]types.Resource)

	resources[resource.ClusterType] = configType.GetClusters()
	resources[resource.RouteType] = configType.GetRoutes()
	resources[resource.ListenerType] = configType.GetListeners()
	resources[resource.SecretType] = secrets
	resources[resource.EndpointType] = endpoints

	return cache.NewSnapshot(version, resources)
}

func NewSecrets(dnsName string, validation interface{}) ([]tls.Secret, error) {
	_, serverCertBytes, _, serverKeyBytes, err := certs.NewCertificate([]string{dnsName}, certs.CertValidity)
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

		if validationContext.GetTrustedCa() == nil {
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

const timeTrackWarning = 100 * time.Millisecond

// defer utils.TimeTrack("func-name",time.Now()).
func TimeTrack(name string, start time.Time) {
	elapsed := time.Since(start)

	metrics.Operation.WithLabelValues(name).Inc()
	metrics.OperationDuration.WithLabelValues(name).Observe(float64(elapsed.Milliseconds()))

	msg := fmt.Sprintf("func: %s, took: %s", name, elapsed)

	if elapsed > timeTrackWarning {
		log.Warn(msg)
	} else {
		log.Debug(msg)
	}
}
