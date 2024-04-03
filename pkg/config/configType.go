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
	"strconv"
	"strings"
	"text/template"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/maksim-paskal/envoy-control-plane/pkg/resources"
	"github.com/maksim-paskal/utils-go"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/anypb"
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

type ConfigType struct { //nolint: revive
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
	// source configmap annotations
	ConfigMapAnnotations map[string]string
	// kubernetes endpoints
	Kubernetes []KubernetesType `yaml:"kubernetes"`
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
	// internal resources
	clusters, routes, listeners, secrets []types.Resource
}

func (c *ConfigType) HasClusterWeights() bool {
	for k := range c.ConfigMapAnnotations {
		if strings.HasPrefix(k, annotationRouteClusterWeight) {
			return true
		}
	}

	return false
}

func (c *ConfigType) GetClusters() []types.Resource {
	return c.clusters
}

func (c *ConfigType) GetRoutes() []types.Resource {
	return c.routes
}

func (c *ConfigType) GetListeners() []types.Resource {
	return c.listeners
}

func (c *ConfigType) GetSecrets() []types.Resource {
	return c.secrets
}

type ClusterWeight struct {
	Value int64
}

// get user defined weights, return nil if not found.
func (c *ConfigType) GetClusterWeight(name string) (*ClusterWeight, error) {
	if w, ok := c.ConfigMapAnnotations[annotationRouteClusterWeight+name]; ok {
		i, err := strconv.ParseUint(w, 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "strconv.ParseUint")
		}

		return &ClusterWeight{Value: int64(i)}, nil
	}

	return nil, nil //nolint: nilnil
}

func (c *ConfigType) SaveResources() error {
	clusters, err := resources.YamlToResources(c.Clusters, cluster.Cluster{})
	if err != nil {
		return errors.Wrap(err, "error parsing clusters")
	}

	routes, err := resources.YamlToResources(c.Routes, route.RouteConfiguration{})
	if err != nil {
		return errors.Wrap(err, "error parsing routes")
	}

	listeners, err := resources.YamlToResources(c.Listeners, listener.Listener{})
	if err != nil {
		return errors.Wrap(err, "error parsing listeners")
	}

	secrets, err := resources.YamlToResources(c.Secrets, tls.Secret{})
	if err != nil {
		return errors.Wrap(err, "error parsing secrets")
	}

	c.clusters = clusters
	c.routes = routes
	c.listeners = listeners
	c.secrets = secrets

	// update cluster weights
	if c.HasClusterWeights() {
		if err := mutateWeightedRoutesInListeners(c, c.listeners); err != nil {
			return errors.Wrap(err, "errors in mutateWeightedRoutesInListeners")
		}

		if err := mutateWeightedRoutes(c, c.routes); err != nil {
			return errors.Wrap(err, "errors in mutateWeightedRoutes")
		}
	}

	// remove all require_client_certificate from listiners
	if *Get().SSLDoNotUseValidation {
		err = filterCertificates(c.listeners)
		if err != nil {
			return errors.Wrap(err, "errors in filterCertificates")
		}
	}

	return nil
}

func ParseConfigYaml(nodeID string, text string, data interface{}) (*ConfigType, error) {
	t := template.New(nodeID)
	templates := template.Must(t.Funcs(utils.GoTemplateFunc(t)).Parse(text))

	var tpl bytes.Buffer

	err := templates.ExecuteTemplate(&tpl, path.Base(nodeID), data)
	if err != nil {
		return nil, errors.Wrap(err, "templates.ExecuteTemplate")
	}

	config := ConfigType{
		VersionLabelKey: "version",
	}

	err = yaml.Unmarshal(tpl.Bytes(), &config)
	if err != nil {
		return nil, errors.Wrap(err, "yaml.Unmarshal")
	}

	return &config, nil
}

var errUnknownClass = errors.New("unknown class")

// remove require_client_certificate from all listeners.
func filterCertificates(listiners []types.Resource) error {
	for _, listiner := range listiners {
		c, ok := listiner.(*listener.Listener)
		if !ok {
			return errUnknownClass
		}

		for _, filterChain := range c.GetFilterChains() {
			s := filterChain.GetTransportSocket()
			if s != nil {
				if s.GetName() == wellknown.TransportSocketTLS {
					r := tls.DownstreamTlsContext{}

					err := s.GetTypedConfig().UnmarshalTo(&r)
					if err != nil {
						return err
					}

					if r.GetRequireClientCertificate() != nil {
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

// routes can be stored in listener filters.
func mutateWeightedRoutesInListeners(configType *ConfigType, listiners []types.Resource) error {
	for _, listiner := range listiners {
		l, ok := listiner.(*listener.Listener)
		if !ok {
			return errUnknownClass
		}

		for _, fc := range l.GetFilterChains() {
			for _, f := range fc.GetFilters() {
				if f.GetName() != wellknown.HTTPConnectionManager {
					continue
				}

				m := hcm.HttpConnectionManager{}

				if err := f.GetTypedConfig().UnmarshalTo(&m); err != nil {
					return errors.Wrap(err, "error unmarshal to HttpConnectionManager")
				}

				if err := mutateWeightedRouteConfiguration(configType, m.GetRouteConfig()); err != nil {
					return errors.Wrap(err, "error mutateWeightedRouteConfiguration")
				}

				pbst, err := anypb.New(&m)
				if err != nil {
					return errors.Wrap(err, "error anypb.New")
				}

				f.ConfigType = &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				}
			}
		}
	}

	return nil
}

func mutateWeightedRoutes(configType *ConfigType, routes []types.Resource) error {
	for _, item := range routes {
		r, ok := item.(*route.RouteConfiguration)
		if !ok {
			return errUnknownClass
		}

		if err := mutateWeightedRouteConfiguration(configType, r); err != nil {
			return errors.Wrap(err, "error mutateWeightedRouteConfiguration")
		}
	}

	return nil
}

func mutateWeightedRouteConfiguration(configType *ConfigType, r *route.RouteConfiguration) error {
	if r == nil {
		return nil
	}

	for _, v := range r.GetVirtualHosts() {
		for _, vr := range v.GetRoutes() {
			if wc := vr.GetRoute().GetWeightedClusters(); wc != nil {
				for _, c := range wc.GetClusters() {
					weight, err := configType.GetClusterWeight(c.GetName())
					if err != nil {
						return err
					}

					if weight != nil && c.GetWeight().GetValue() != uint32(weight.Value) {
						log.Warnf("mutateWeightedRoutes: %s, weight: %d -> %d", c.GetName(), c.GetWeight().GetValue(), weight)

						c.Weight = &wrappers.UInt32Value{Value: uint32(weight.Value)}
					}
				}
			}
		}
	}

	return nil
}
