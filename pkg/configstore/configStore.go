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
	"github.com/maksim-paskal/envoy-control-plane/pkg/endpointstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConfigStoreStateRUN int = iota
	ConfigStoreStateSTOP
)

var StoreMap = new(sync.Map)

type ConfigStore struct {
	Version             string
	Config              *appConfig.ConfigType
	ep                  *endpointstore.EndpointsStore
	KubernetesEndpoints sync.Map
	configEndpoints     map[string][]*endpoint.LocalityLbEndpoints
	lastEndpoints       []types.ResourceWithTTL
	LastEndpointsArray  []string
	ConfigStoreState    int
	log                 *log.Entry
	mutex               sync.Mutex
	ctx                 context.Context
	secrets             []tls.Secret
}

func New(config *appConfig.ConfigType, ep *endpointstore.EndpointsStore) (*ConfigStore, error) {
	cs := ConfigStore{
		Config:           config,
		ep:               ep,
		ctx:              context.Background(),
		ConfigStoreState: ConfigStoreStateRUN,
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

	namespaceSearch := ""

	if *appConfig.Get().WatchNamespaced {
		namespaceSearch = *appConfig.Get().Namespace
	}

	pods, err := api.Clientset.CoreV1().Pods(namespaceSearch).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		cs.log.WithError(err).Error()
	}

	log.Infof("Found %d pods", len(pods.Items))

	for i := range pods.Items {
		cs.loadEndpoint(&pods.Items[i])
	}

	cs.saveLastEndpoints()

	if err = cs.LoadNewSecrets(); err != nil {
		return nil, errors.Wrap(err, "error in LoadNewSecrets")
	}

	cs.Push()

	return &cs, nil
}

func (cs *ConfigStore) NewPod(pod *v1.Pod) {
	if cs.ConfigStoreState == ConfigStoreStateSTOP {
		return
	}

	cs.loadEndpoint(pod)
	cs.saveLastEndpoints()
}

func (cs *ConfigStore) DeletePod(pod *v1.Pod) {
	if cs.ConfigStoreState == ConfigStoreStateSTOP {
		return
	}

	cs.KubernetesEndpoints.Delete(pod.Name)

	cs.saveLastEndpoints()
}

func (cs *ConfigStore) Push() {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

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

	cs.log.WithField("version", cs.Version).Infof("pushed")
}

func (cs *ConfigStore) loadEndpoint(pod *v1.Pod) {
	// load only valid pods with ip and node
	if len(pod.Status.PodIP) > 0 && len(pod.Spec.NodeName) > 0 {
		podInfo := cs.podInfo(pod)

		cs.log.Debugf("pod=%s,namespace=%s,podInfo=%+v", pod.Name, pod.Namespace, podInfo)

		if pod.DeletionTimestamp != nil {
			cs.KubernetesEndpoints.Delete(pod.Name)
		} else if podInfo.check {
			cs.KubernetesEndpoints.Store(pod.Name, podInfo)
		}
	} else {
		log.Debugf("pod %s not valid", pod.Name)
	}
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

	cs.KubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
		info, ok := value.(CheckPodResult)
		if !ok {
			cs.log.WithError(errAssertion).Fatal("value.(CheckPodResult)")
		}

		// add endpoint only if ready
		if info.ready && len(info.podIP) > 0 && len(info.nodeZone) > 0 {
			nodeLocality := &core.Locality{
				Zone: info.nodeZone,
			}

			priority := uint32(0)

			if info.priority > 0 {
				priority = info.priority
			}

			healthCheckConfig := &endpoint.Endpoint_HealthCheckConfig{}

			if info.healthCheckPort > 0 {
				healthCheckConfig.PortValue = info.healthCheckPort
			}
			// add element to publishEpArray
			lbEndpoints[info.clusterName] = append(lbEndpoints[info.clusterName], &endpoint.LocalityLbEndpoints{
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
										Address:  info.podIP,
										PortSpecifier: &core.SocketAddress_PortValue{
											PortValue: info.port,
										},
									},
								},
							},
						},
					},
				}},
			})
		}

		return true
	})

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

	if !reflect.DeepEqual(cs.LastEndpointsArray, publishEpArray) {
		cs.lastEndpoints = publishEp
		cs.LastEndpointsArray = publishEpArray
		cs.log.Debug("new endpoints")
		// endpoints changes
		cs.Push()
	}
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

func (cs *ConfigStore) podInfo(pod *v1.Pod) CheckPodResult {
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

				// get zone from saved endpoint
				ep, ok := cs.KubernetesEndpoints.Load(pod.Name)
				if ok {
					saved, ok := ep.(CheckPodResult)
					if !ok {
						cs.log.WithError(errAssertion).Fatal("ep.(CheckPodResult)")
					}

					result.nodeZone = saved.nodeZone
				}

				if len(result.nodeZone) == 0 {
					zone := ""

					nodeInfo, err := cs.getNode(pod.Spec.NodeName)
					if err != nil {
						log.WithError(err).Error()
					} else {
						zone = nodeInfo.Labels[*appConfig.Get().NodeZoneLabel]
					}

					if len(zone) == 0 {
						zone = "unknown"
					}

					result.nodeZone = zone
				}

				return result
			}
		}
	}

	return CheckPodResult{check: false}
}

func (cs *ConfigStore) Stop() {
	cs.log.Info("stop")
	cs.ConfigStoreState = ConfigStoreStateSTOP
}

func (cs *ConfigStore) getNode(nodeName string) (*v1.Node, error) {
	nodeInfo, err := api.Clientset.CoreV1().Nodes().Get(cs.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "cs.ep.clientset.CoreV1().Nodes().Get")
	}

	return nodeInfo, nil
}

func (cs *ConfigStore) Sync() {
	if cs.ConfigStoreState == ConfigStoreStateSTOP {
		return
	}

	if cs.lastEndpoints != nil {
		snap, err := controlplane.SnapshotCache.GetSnapshot(cs.Config.ID)
		if err != nil {
			log.WithError(err).Warn()
		}

		snapVersion := snap.GetVersion(resource.EndpointType)

		if len(snapVersion) > 0 && snapVersion != cs.Version {
			log.Warnf("nodeID=%s,version not match %s,%s", cs.Config.ID, snapVersion, cs.Version)

			cs.lastEndpoints = nil
			cs.LastEndpointsArray = nil

			cs.saveLastEndpoints()
		}
	}
}
