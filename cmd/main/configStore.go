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
	"fmt"
	"net"
	"reflect"
	"sort"
	"sync"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConfigStoreStateRun int = iota
	ConfigStoreStateStop
)

type ConfigStore struct {
	config              *ConfigType
	ep                  *EndpointsStore
	kubernetesEndpoints sync.Map
	configEndpoints     map[string][]*endpoint.LocalityLbEndpoints
	lastEndpoints       []types.Resource
	lastEndpointsArray  []string
	ConfigStoreState    int
	log                 *log.Entry
}

func newConfigStore(config *ConfigType, ep *EndpointsStore) *ConfigStore {
	cs := ConfigStore{
		config:           config,
		ep:               ep,
		ConfigStoreState: ConfigStoreStateRun,
		log: log.WithFields(log.Fields{
			"type":   "ConfigStore",
			"nodeID": config.ID,
		}),
	}

	if log.GetLevel() >= log.DebugLevel {
		obj, err := yaml.Marshal(config)
		if err != nil {
			log.Error(err)
		}
		cs.log.Debugf("loaded config: \n%s", string(obj))
	}

	var err error
	cs.configEndpoints, err = cs.getConfigEndpoints()
	if err != nil {
		log.Error(err)
	}

	for _, v := range ep.informer.GetStore().List() {
		pod := v.(*v1.Pod)
		cs.loadEndpoint(pod)
	}
	cs.saveLastEndpoints()

	cs.push()

	return &cs
}

func (cs *ConfigStore) NewPod(pod *v1.Pod) {
	if cs.ConfigStoreState == ConfigStoreStateStop {
		return
	}
	cs.loadEndpoint(pod)
	cs.saveLastEndpoints()
}

func (cs *ConfigStore) DeletePod() {
	if cs.ConfigStoreState == ConfigStoreStateStop {
		return
	}
	cs.saveLastEndpoints()
}

func (cs *ConfigStore) push() {
	version := uuid.New().String()
	snap, err := getConfigSnapshot(version, cs.config, cs.lastEndpoints)
	if err != nil {
		cs.log.Error(err)

		return
	}
	err = snapshotCache.SetSnapshot(cs.config.ID, snap)
	if err != nil {
		cs.log.Error(err)

		return
	}
	cs.log.Infof("pushed,version=%s", version)
}

func (cs *ConfigStore) loadEndpoint(pod *v1.Pod) {
	// load only valid pods with ip
	if len(pod.Status.PodIP) > 0 {
		podInfo := cs.podInfo(pod)

		cs.log.Debugf("pod=%s,namespace=%s,podInfo=%+v", pod.Name, pod.Namespace, podInfo)

		if podInfo.check {
			cs.kubernetesEndpoints.Store(pod.Name, podInfo)
		}
	}
}

func (cs *ConfigStore) getConfigEndpoints() (map[string][]*endpoint.LocalityLbEndpoints, error) {
	endpoints, err := yamlToResources(cs.config.Endpoints, api.ClusterLoadAssignment{})
	if err != nil {
		cs.log.Error(err)

		return nil, err
	}

	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)

	for _, ep := range endpoints {
		fixed := ep.(*api.ClusterLoadAssignment)

		lbEndpoints[fixed.GetClusterName()] = append(lbEndpoints[fixed.GetClusterName()], fixed.GetEndpoints()...)
	}

	return lbEndpoints, nil
}

// save endpoints.
func (cs *ConfigStore) saveLastEndpoints() {
	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)
	// copy map
	for key, value := range cs.configEndpoints {
		lbEndpoints[key] = value
	}

	cs.kubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
		info := value.(checkPodResult)

		if len(info.podIP) > 0 && len(info.nodeZone) > 0 {
			healthStatus := core.HealthStatus_UNHEALTHY

			if info.ready {
				healthStatus = core.HealthStatus_HEALTHY
			}

			if info.podDeletion {
				healthStatus = core.HealthStatus_DRAINING
			}

			nodeLocality := &core.Locality{
				Zone: info.nodeZone,
			}

			var priority uint32 = 0

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
					HealthStatus: healthStatus,
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

	var isInvalidIP bool = false
	publishEp := []types.Resource{}
	publishEpArray := []string{} // for reflect.DeepEqual

	for clusterName, ep := range lbEndpoints {
		for _, value1 := range ep {
			for _, value2 := range value1.LbEndpoints {
				address := value2.GetEndpoint().GetAddress().GetSocketAddress().Address

				publishEpArray = append(publishEpArray, fmt.Sprintf(
					"%s|%s|%d|%d|%s|%d|%d",
					clusterName,
					value1.Locality.GetZone(),
					value1.Priority,
					value2.HealthStatus,
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
		publishEp = append(publishEp, &api.ClusterLoadAssignment{
			ClusterName: clusterName,
			Endpoints:   ep,
		})
	}
	if isInvalidIP {
		return
	}

	// reflect.DeepEqual only on sorted values
	sort.Strings(publishEpArray)

	if !reflect.DeepEqual(cs.lastEndpointsArray, publishEpArray) {
		cs.lastEndpoints = publishEp
		cs.lastEndpointsArray = publishEpArray
		cs.log.Debug("new endpoints")
		// endpoints changes
		cs.push()
	}
}

type checkPodResult struct {
	check           bool
	podDeletion     bool
	clusterName     string
	podIP           string
	port            uint32
	ready           bool
	nodeZone        string
	priority        uint32
	healthCheckPort uint32
}

func (cs *ConfigStore) podInfo(pod *v1.Pod) checkPodResult {
	for _, config := range cs.config.Kubernetes {
		if config.Namespace == pod.Namespace {
			labelsFound := 0
			for k2, v2 := range pod.Labels {
				if config.Selector[k2] == v2 {
					labelsFound++
				}
			}
			if labelsFound == len(config.Selector) {
				ready := false
				podDeletion := false

				if pod.DeletionTimestamp != nil {
					podDeletion = true
				}

				if pod.Status.Phase == v1.PodRunning {
					for _, v := range pod.Status.Conditions {
						if v.Type == v1.PodReady && v.Status == "True" {
							ready = true
						}
					}
				}

				result := checkPodResult{
					check:           true,
					clusterName:     config.ClusterName,
					podIP:           pod.Status.PodIP,
					ready:           ready,
					podDeletion:     podDeletion,
					healthCheckPort: config.HealthCheckPort,
					port:            config.Port,
					priority:        config.Priority,
				}

				ep, ok := cs.kubernetesEndpoints.Load(pod.Name)
				if ok {
					saved := ep.(checkPodResult)
					result.nodeZone = saved.nodeZone
				}

				if len(result.nodeZone) == 0 && len(pod.Spec.NodeName) > 0 {
					nodeInfo := cs.getNode(pod.Spec.NodeName)
					zone := nodeInfo.Labels[*appConfig.NodeZoneLabel]
					if len(zone) == 0 {
						zone = "unknown"
					}
					result.nodeZone = zone
				}

				return result
			}
		}
	}

	return checkPodResult{check: false}
}

func (cs *ConfigStore) Stop() {
	cs.log.Info("stop")
	cs.ConfigStoreState = ConfigStoreStateStop
}

func (cs *ConfigStore) getNode(nodeName string) *v1.Node {
	nodeInfo, err := cs.ep.clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		cs.log.Error(err)
	}

	return nodeInfo
}

func (cs *ConfigStore) Sync() {
	if cs.ConfigStoreState == ConfigStoreStateStop {
		return
	}

	cs.kubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
		info := value.(checkPodResult)
		if info.podDeletion {
			log.Debugf("delete drain pod=%s", key)
			cs.kubernetesEndpoints.Delete(key)
		}

		return true
	})
	cs.saveLastEndpoints()
}
