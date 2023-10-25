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
package configstore

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sort"
	"sync"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/google/uuid"
	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	appConfig "github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	"github.com/maksim-paskal/envoy-control-plane/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
)

var StoreMap = new(sync.Map)

const (
	defaultZone         = "unknown"
	envoyMetaPodName    = "k8s.pod.name"
	envoyMetaPodLabels  = "k8s.pod.labels."
	envoyMetaEndpointIP = "k8s.endpoint.ip"
	envoyMetaNodeName   = "k8s.node.name"
	podLabelIgnore      = "pod-template-hash"
)

type ConfigStore struct {
	Version            string
	Config             *appConfig.ConfigType
	configEndpoints    map[string][]*endpoint.LocalityLbEndpoints
	lastEndpoints      []types.Resource
	lastEndpointsArray []string
	log                *log.Entry
	mutex              sync.Mutex
	secrets            []tls.Secret
	isStoped           *atomic.Bool
}

func New(ctx context.Context, config *appConfig.ConfigType) (*ConfigStore, error) {
	cs := ConfigStore{
		Config:   config,
		isStoped: atomic.NewBool(false),
		log: log.WithFields(log.Fields{
			"type":   "ConfigStore",
			"nodeID": config.ID,
		}),
	}

	if log.GetLevel() >= log.DebugLevel {
		obj, err := yaml.Marshal(config)
		if err != nil {
			cs.log.WithError(err).Error()
		}

		cs.log.Debugf("loaded config: \n%s", string(obj))
	}

	var err error
	cs.configEndpoints, err = cs.getConfigEndpoints()

	if err != nil {
		cs.log.WithError(err).Error()
	}

	if err = cs.LoadNewSecrets(); err != nil {
		return nil, errors.Wrap(err, "error in LoadNewSecrets")
	}

	cs.saveLastEndpoints(ctx)

	return &cs, nil
}

func (cs *ConfigStore) hasStoped() bool {
	return cs.isStoped.Load()
}

func (cs *ConfigStore) NewPod(ctx context.Context, _ *corev1.Pod) {
	if cs.hasStoped() {
		return
	}

	cs.saveLastEndpoints(ctx)
}

func (cs *ConfigStore) NewEndpoint(ctx context.Context, _ *corev1.Endpoints) {
	cs.NewPod(ctx, nil)
}

func (cs *ConfigStore) DeletePod(ctx context.Context, _ *corev1.Pod) {
	cs.NewPod(ctx, nil)
}

func (cs *ConfigStore) Push(ctx context.Context, reason string) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	metrics.ConfigmapsstorePush.Inc()

	for {
		newVersion := uuid.New().String()
		if newVersion != cs.Version {
			cs.Version = newVersion

			break
		}
	}

	snap, err := utils.GetConfigSnapshot(cs.Version, cs.Config, cs.lastEndpoints, cs.secrets)
	if err != nil {
		cs.log.WithError(err).Error()

		return
	}

	err = controlplane.SnapshotCache.SetSnapshot(ctx, cs.Config.ID, snap)

	if err != nil {
		cs.log.WithError(err).Error()

		return
	}

	cs.log.WithField("version", cs.Version).Infof("pushed, reason=%s", reason)
}

func (cs *ConfigStore) getConfigEndpoints() (map[string][]*endpoint.LocalityLbEndpoints, error) {
	endpoints, err := utils.YamlToResources(cs.Config.Endpoints, endpoint.ClusterLoadAssignment{})
	if err != nil {
		return nil, err
	}

	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)

	for _, ep := range endpoints {
		fixed, ok := ep.(*endpoint.ClusterLoadAssignment)
		if !ok {
			cs.log.WithError(errAssertion).Fatal("ep.(*endpoint.ClusterLoadAssignment)")
		}

		lbEndpoints[fixed.GetClusterName()] = append(lbEndpoints[fixed.GetClusterName()], fixed.GetEndpoints()...)
	}

	return lbEndpoints, nil
}

// create new secrets.
func (cs *ConfigStore) LoadNewSecrets() error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	secrets, err := utils.NewSecrets(cs.Config.Name, cs.Config.Validation)
	if err != nil {
		return errors.Wrap(err, "can not create secrets")
	}

	cs.secrets = secrets

	return nil
}

func (cs *ConfigStore) getEndpointLocality(node string) *core.Locality {
	nodeInfo, err := api.GetNode(node)
	if err != nil {
		log.WithError(err).Errorf("can not get node info for %s", node)

		return &core.Locality{
			Zone: defaultZone,
		}
	}

	zone := nodeInfo.Labels[*appConfig.Get().NodeZoneLabel]

	if len(zone) == 0 {
		zone = defaultZone
	}

	return &core.Locality{
		Zone: zone,
	}
}

type envoyEndpoint struct {
	IsCanary bool
	Node     string
	Address  string
	Item     appConfig.KubernetesType
	Metadata map[string]string
}

func (e *envoyEndpoint) SetNode(address corev1.EndpointAddress) {
	if address.NodeName != nil {
		e.Node = *address.NodeName
	}
}

func (cs *ConfigStore) getEnvoyLocalityLbEndpoint(envoyEndpoint *envoyEndpoint) *endpoint.LocalityLbEndpoints { //nolint:lll
	priority := uint32(0)

	if envoyEndpoint.Item.Priority > 0 {
		priority = envoyEndpoint.Item.Priority
	}

	healthCheckConfig := &endpoint.Endpoint_HealthCheckConfig{}

	if envoyEndpoint.Item.HealthCheckPort > 0 {
		healthCheckConfig.PortValue = envoyEndpoint.Item.HealthCheckPort
	}

	endpointStage := "main"

	if envoyEndpoint.IsCanary {
		endpointStage = "canary"
	}

	metadataEnvoyLB := make(map[string]*structpb.Value)

	metadataEnvoyLB["canary"] = &structpb.Value{
		Kind: &structpb.Value_BoolValue{
			BoolValue: envoyEndpoint.IsCanary,
		},
	}

	metadataEnvoyLB["stage"] = &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: endpointStage,
		},
	}

	// add all metadata
	for k, v := range envoyEndpoint.Metadata {
		metadataEnvoyLB[k] = &structpb.Value{
			Kind: &structpb.Value_StringValue{
				StringValue: v,
			},
		}
	}

	return &endpoint.LocalityLbEndpoints{
		Locality: cs.getEndpointLocality(envoyEndpoint.Node),
		Priority: priority,
		LbEndpoints: []*endpoint.LbEndpoint{{
			Metadata: &core.Metadata{
				FilterMetadata: map[string]*structpb.Struct{
					"envoy.lb": {
						Fields: metadataEnvoyLB,
					},
				},
			},
			HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					HealthCheckConfig: healthCheckConfig,
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Protocol: core.SocketAddress_TCP,
								Address:  envoyEndpoint.Address,
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: envoyEndpoint.Item.Port,
								},
							},
						},
					},
				},
			},
		}},
	}
}

// return true if pod is ready.
func (cs *ConfigStore) isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodRunning {
		for _, podCondition := range pod.Status.Conditions {
			if podCondition.Type == corev1.PodReady && podCondition.Status == "True" {
				return true
			}
		}
	}

	return false
}

func (cs *ConfigStore) getLocalityLbEndpoints() (map[string][]*endpoint.LocalityLbEndpoints, error) {
	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)

	// loading endpoints from pods selector
	for _, kubernetes := range cs.Config.Kubernetes {
		// get endpoint only for objects with selector
		if kubernetes.Selector == nil {
			continue
		}

		pods, err := api.ListPods(kubernetes.Selector)
		if err != nil {
			return nil, errors.Wrap(err, "error getting pods")
		}

		for _, pod := range pods {
			// ignore pod if it has no IP or no node
			if len(pod.Status.PodIP) == 0 || len(pod.Spec.NodeName) == 0 {
				continue
			}

			// ignore pod if deleted
			if pod.DeletionTimestamp != nil {
				continue
			}

			// ignore pod if not ready
			if !cs.isPodReady(pod) {
				continue
			}

			// get envoy endpoint
			lbEndpoints[kubernetes.ClusterName] = append(lbEndpoints[kubernetes.ClusterName], cs.getEnvoyLocalityLbEndpoint(&envoyEndpoint{ //nolint:lll
				IsCanary: false,
				Node:     pod.Spec.NodeName,
				Address:  pod.Status.PodIP,
				Item:     kubernetes,
				Metadata: cs.getEnvoyMetaFromPod(pod),
			},
			))
		}
	}

	// loading endpoints from service
	for _, kubernetes := range cs.Config.Kubernetes {
		// get endpoint with service name
		if len(kubernetes.Service) == 0 {
			continue
		}

		// get endpoints by service name
		endpoints, err := api.GetEndpoint(kubernetes.Service)
		if err != nil {
			return nil, errors.Wrap(err, "error getting endpoints")
		}

		// service not found
		if endpoints == nil {
			log.Warnf("service not found: %s", kubernetes.Service)

			continue
		}

		for _, subset := range endpoints.Subsets {
			for _, address := range subset.Addresses {
				newEp := &envoyEndpoint{
					IsCanary: false,
					Address:  address.IP,
					Item:     kubernetes,
					Metadata: cs.getEnvoyMetaFromEndpoint(address),
				}

				newEp.SetNode(address)

				// get envoy endpoint
				lbEndpoints[kubernetes.ClusterName] = append(
					lbEndpoints[kubernetes.ClusterName],
					cs.getEnvoyLocalityLbEndpoint(newEp),
				)
			}
		}

		// get canary endpoints by service name
		endpointsCanary, err := api.GetEndpoint(kubernetes.Service + appConfig.CanarySuffix)
		if err != nil {
			return nil, errors.Wrap(err, "error getting endpoints")
		}

		// service not found
		if endpointsCanary == nil {
			log.Warnf("service not found: %s", kubernetes.Service)

			continue
		}

		for _, subset := range endpointsCanary.Subsets {
			for _, address := range subset.Addresses {
				newEp := &envoyEndpoint{
					IsCanary: true,
					Address:  address.IP,
					Item:     kubernetes,
					Metadata: cs.getEnvoyMetaFromEndpoint(address),
				}

				newEp.SetNode(address)

				// get envoy endpoint
				lbEndpoints[kubernetes.ClusterName] = append(
					lbEndpoints[kubernetes.ClusterName],
					cs.getEnvoyLocalityLbEndpoint(newEp),
				)
			}
		}
	}

	return lbEndpoints, nil
}

func (cs *ConfigStore) getEnvoyMetaFromPod(pod *corev1.Pod) map[string]string {
	labels := make(map[string]string)

	// add pod labels to envoy metadata
	if pod.Labels != nil {
		for k, v := range pod.Labels {
			if k != podLabelIgnore {
				labels[envoyMetaPodLabels+k] = v
			}
		}
	}

	labels[envoyMetaPodName] = pod.Name
	labels[envoyMetaEndpointIP] = pod.Status.PodIP
	labels[envoyMetaNodeName] = pod.Spec.NodeName

	return labels
}

func (cs *ConfigStore) getEnvoyMetaFromEndpoint(address corev1.EndpointAddress) map[string]string {
	labels := make(map[string]string)

	if address.TargetRef != nil && address.TargetRef.Kind == "Pod" {
		// add pod labels to envoy metadata
		pod, err := api.GetPod(address.TargetRef.Namespace, address.TargetRef.Name)
		if err != nil {
			log.WithError(err).Error("error getting pod")
		} else if pod != nil && pod.Labels != nil {
			for k, v := range pod.Labels {
				if k != podLabelIgnore {
					labels[envoyMetaPodLabels+k] = v
				}
			}
		}

		labels[envoyMetaPodName] = address.TargetRef.Name
	}

	if address.NodeName != nil {
		labels[envoyMetaNodeName] = *address.NodeName
	}

	labels[envoyMetaEndpointIP] = address.IP

	return labels
}

// save endpoints.
func (cs *ConfigStore) saveLastEndpoints(ctx context.Context) {
	defer utils.TimeTrack("saveLastEndpoints", time.Now())

	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)
	// copy map
	for key, value := range cs.configEndpoints {
		lbEndpoints[key] = value
	}

	endpoints, err := cs.getLocalityLbEndpoints()
	if err != nil {
		log.WithError(err).Error(err)

		return
	}

	// append endpoints
	for key, value := range endpoints {
		lbEndpoints[key] = append(lbEndpoints[key], value...)
	}

	isInvalidIP := false
	publishEp := []types.Resource{}
	publishEpArray := []string{} // for reflect.DeepEqual

	for clusterName, ep := range lbEndpoints {
		for _, value1 := range ep {
			for _, value2 := range value1.GetLbEndpoints() {
				address := value2.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()

				publishEpArray = append(publishEpArray, fmt.Sprintf(
					"%s|%s|%d|%s|%d|%d",
					clusterName,
					value1.GetLocality().GetZone(),
					value1.GetPriority(),
					value2.GetEndpoint().GetAddress().GetSocketAddress().GetAddress(),
					value2.GetEndpoint().GetAddress().GetSocketAddress().GetPortValue(),
					value2.GetEndpoint().GetHealthCheckConfig().GetPortValue(),
				))

				if net.ParseIP(address) == nil {
					isInvalidIP = true

					cs.log.Errorf("clusterName=%s,ip=%s is invalid", clusterName, address)
				}
			}
		}

		clusterLoadAssignment := endpoint.ClusterLoadAssignment{
			ClusterName: clusterName,
			Endpoints:   ep,
		}

		publishEp = append(publishEp, &clusterLoadAssignment)
	}

	if isInvalidIP {
		log.WithError(errInvalidIP).Warn()

		return
	}

	// reflect.DeepEqual only on sorted values
	sort.Strings(publishEpArray)

	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if !reflect.DeepEqual(cs.lastEndpointsArray, publishEpArray) {
		cs.lastEndpoints = publishEp
		cs.lastEndpointsArray = publishEpArray

		// endpoints changes
		go cs.Push(ctx, "new endpoints")
	}
}

func (cs *ConfigStore) GetLastEndpoints() []string {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	return cs.lastEndpointsArray
}

func (cs *ConfigStore) Stop() {
	cs.log.Info("stop")
	cs.isStoped.Store(true)
}

func (cs *ConfigStore) Sync(ctx context.Context) {
	if cs.hasStoped() {
		return
	}

	cs.saveLastEndpoints(ctx)

	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if cs.lastEndpoints != nil {
		snap, err := controlplane.SnapshotCache.GetSnapshot(cs.Config.ID)
		if err != nil {
			log.WithError(err).Warn()
		}

		snapVersion := snap.GetVersion(resource.EndpointType)

		if len(snapVersion) > 0 && snapVersion != cs.Version {
			log.Warnf("nodeID=%s,version not match %s,%s", cs.Config.ID, snapVersion, cs.Version)

			cs.lastEndpoints = nil
			cs.lastEndpointsArray = nil

			go cs.saveLastEndpoints(ctx)
		}
	}
}
