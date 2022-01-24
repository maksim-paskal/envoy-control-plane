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
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
)

var StoreMap = new(sync.Map)

type ConfigStore struct {
	Version            string
	Config             *appConfig.ConfigType
	configEndpoints    map[string][]*endpoint.LocalityLbEndpoints
	lastEndpoints      []types.ResourceWithTTL
	lastEndpointsArray []string
	log                *log.Entry
	mutex              sync.Mutex
	ctx                context.Context
	secrets            []tls.Secret
	isStoped           *atomic.Bool
}

func New(config *appConfig.ConfigType) (*ConfigStore, error) {
	cs := ConfigStore{
		Config:   config,
		ctx:      context.Background(),
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

	cs.saveLastEndpoints()

	return &cs, nil
}

func (cs *ConfigStore) hasStoped() bool {
	return cs.isStoped.Load()
}

func (cs *ConfigStore) NewPod(pod *v1.Pod) {
	if cs.hasStoped() {
		return
	}

	cs.saveLastEndpoints()
}

func (cs *ConfigStore) DeletePod(pod *v1.Pod) {
	if cs.hasStoped() {
		return
	}

	cs.saveLastEndpoints()
}

func (cs *ConfigStore) Push(reason string) {
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

	err = controlplane.SnapshotCache.SetSnapshot(cs.ctx, cs.Config.ID, snap)

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
		fixed, ok := ep.Resource.(*endpoint.ClusterLoadAssignment)
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

// save endpoints.
func (cs *ConfigStore) saveLastEndpoints() {
	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)
	// copy map
	for key, value := range cs.configEndpoints {
		lbEndpoints[key] = value
	}

	pods, err := api.ListPods()
	if err != nil {
		log.WithError(err).Error(err)
	}

	for _, pod := range pods {
		// if pod has no IP or node
		if len(pod.Status.PodIP) == 0 || len(pod.Spec.NodeName) == 0 {
			continue
		}

		// pod deleted
		if pod.DeletionTimestamp != nil {
			continue
		}

		podInfo, err := cs.podInfo(pod)
		if err != nil {
			log.WithError(err).Errorf("error getting pod %s", cs.getPodID(pod))

			break
		}

		// if pod does not assign to cluster
		if !podInfo.check {
			continue
		}

		// if pod does not ready
		if !podInfo.ready || len(podInfo.podIP) == 0 || len(podInfo.nodeZone) == 0 {
			continue
		}

		nodeLocality := &core.Locality{
			Zone: podInfo.nodeZone,
		}

		priority := uint32(0)

		if podInfo.priority > 0 {
			priority = podInfo.priority
		}

		healthCheckConfig := &endpoint.Endpoint_HealthCheckConfig{}

		if podInfo.healthCheckPort > 0 {
			healthCheckConfig.PortValue = podInfo.healthCheckPort
		}
		// add element to publishEpArray
		lbEndpoints[podInfo.clusterName] = append(lbEndpoints[podInfo.clusterName], &endpoint.LocalityLbEndpoints{
			Locality: nodeLocality,
			Priority: priority,
			LbEndpoints: []*endpoint.LbEndpoint{{
				HostIdentifier: &endpoint.LbEndpoint_Endpoint{
					Endpoint: &endpoint.Endpoint{
						HealthCheckConfig: healthCheckConfig,
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
	}

	isInvalidIP := false
	publishEp := []types.ResourceWithTTL{}
	publishEpArray := []string{} // for reflect.DeepEqual

	for clusterName, ep := range lbEndpoints {
		for _, value1 := range ep {
			for _, value2 := range value1.LbEndpoints {
				address := value2.GetEndpoint().GetAddress().GetSocketAddress().Address

				publishEpArray = append(publishEpArray, fmt.Sprintf(
					"%s|%s|%d|%s|%d|%d",
					clusterName,
					value1.Locality.GetZone(),
					value1.Priority,
					value2.GetEndpoint().GetAddress().GetSocketAddress().Address,
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

		publishEp = append(publishEp, types.ResourceWithTTL{
			Resource: &clusterLoadAssignment,
			TTL:      appConfig.Get().EndpointTTL,
		})
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
		go cs.Push("new endpoints")
	}
}

func (cs *ConfigStore) GetLastEndpoints() []string {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	return cs.lastEndpointsArray
}

//nolint:maligned
type CheckPodResult struct {
	check           bool
	clusterName     string
	podIP           string
	podName         string
	podNamespace    string
	port            uint32
	ready           bool
	nodeZone        string
	priority        uint32
	healthCheckPort uint32
}

func (cs *ConfigStore) podInfo(pod *v1.Pod) (CheckPodResult, error) {
	for _, config := range cs.Config.Kubernetes {
		if config.Namespace == pod.Namespace {
			labelsFound := 0

			for k2, v2 := range pod.Labels {
				if config.Selector[k2] == v2 {
					labelsFound++
				}
			}

			if labelsFound == len(config.Selector) {
				ready := false

				if pod.Status.Phase == v1.PodRunning {
					for _, v := range pod.Status.Conditions {
						if v.Type == v1.PodReady && v.Status == "True" {
							ready = true
						}
					}
				}

				result := CheckPodResult{
					check:           true,
					clusterName:     config.ClusterName,
					podIP:           pod.Status.PodIP,
					podName:         pod.Name,
					podNamespace:    pod.Namespace,
					ready:           ready,
					healthCheckPort: config.HealthCheckPort,
					port:            config.Port,
					priority:        config.Priority,
				}

				nodeInfo, err := api.GetNode(pod.Spec.NodeName)
				if err != nil {
					return result, err
				}

				zone := nodeInfo.Labels[*appConfig.Get().NodeZoneLabel]
				if len(zone) == 0 {
					zone = "unknown"
				}

				result.nodeZone = zone

				return result, nil
			}
		}
	}

	return CheckPodResult{check: false}, nil
}

func (cs *ConfigStore) Stop() {
	cs.log.Info("stop")
	cs.isStoped.Store(true)
}

func (cs *ConfigStore) Sync() {
	if cs.hasStoped() {
		return
	}

	cs.saveLastEndpoints()

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

			go cs.saveLastEndpoints()
		}
	}
}

func (cs *ConfigStore) getPodID(pod *v1.Pod) string {
	return fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)
}
