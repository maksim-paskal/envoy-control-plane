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
	"net"
	"reflect"
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
	lastEndpoints       []types.Resource
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
			"nodeId": config.Id,
		}),
	}

	if log.GetLevel() >= log.DebugLevel {
		obj, _ := yaml.Marshal(config)
		cs.log.Debugf("loaded config: \n%s", string(obj))
	}

	for _, v := range ep.informer.GetStore().List() {
		pod := v.(*v1.Pod)
		cs.LoadEndpoint(pod)
	}
	cs.saveLastEndpoints()

	cs.Push()
	return &cs
}

func (cs *ConfigStore) newPod(pod *v1.Pod) {
	if cs.ConfigStoreState == ConfigStoreStateStop {
		return
	}
	cs.LoadEndpoint(pod)
	cs.saveLastEndpoints()
}

func (cs *ConfigStore) Push() {
	version := uuid.New().String()
	snap, err := getConfigSnapshot(version, cs.config, cs.lastEndpoints)
	if err != nil {
		cs.log.Error(err)
		return
	}
	err = snapshotCache.SetSnapshot(cs.config.Id, snap)
	if err != nil {
		cs.log.Error(err)
		return
	}
	cs.log.Infof("pushed,version=%s", version)
}
func (cs *ConfigStore) LoadEndpoint(pod *v1.Pod) {
	podInfo := cs.podInfo(pod)

	cs.log.Debugf("pod=%s,namespace=%s,podInfo=%+v", pod.Name, pod.Namespace, podInfo)

	if podInfo.check {
		cs.kubernetesEndpoints.Store(pod.Name, podInfo)
	}
}

// save endpoints
func (cs *ConfigStore) saveLastEndpoints() {
	endpoints, err := yamlToResources(cs.config.Endpoints, api.ClusterLoadAssignment{})
	if err != nil {
		cs.log.Error(err)
		return
	}
	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)

	for _, ep := range endpoints {
		fixed := ep.(*api.ClusterLoadAssignment)

		lbEndpoints[fixed.GetClusterName()] = append(lbEndpoints[fixed.GetClusterName()], fixed.GetEndpoints()...)
	}

	cs.kubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
		info := value.(checkPodResult)
		if info.ready {
			nodeLocality := &core.Locality{}

			if len(info.nodeZone) > 0 {
				nodeLocality.Zone = info.nodeZone
			}

			var priority uint32 = 0

			if info.priority > 0 {
				priority = info.priority
			}

			healthCheckConfig := &endpoint.Endpoint_HealthCheckConfig{}

			if info.healthCheckPort > 0 {
				healthCheckConfig.PortValue = info.healthCheckPort
			}

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

	var isInvalidIP bool = false
	var publishEp []types.Resource
	for clusterName, ep := range lbEndpoints {
		for _, value1 := range ep {
			for _, value2 := range value1.LbEndpoints {
				address := value2.GetEndpoint().GetAddress().GetSocketAddress().Address
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
	if !reflect.DeepEqual(cs.lastEndpoints, publishEp) {
		cs.lastEndpoints = publishEp
		cs.log.Debug("new endpoints")
		// endpoints changes
		cs.Push()
	}
}

type checkPodResult struct {
	check           bool
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
					labelsFound = labelsFound + 1
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

				result := checkPodResult{
					check:           true,
					clusterName:     config.ClusterName,
					podIP:           pod.Status.PodIP,
					ready:           ready,
					healthCheckPort: config.HealthCheckPort,
					port:            config.Port,
					priority:        config.Priority,
				}
				if ready && *appConfig.ZoneLabels {
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

	snapshotCache.ClearSnapshot(cs.config.Id)
}

func (cs *ConfigStore) getNode(nodeName string) *v1.Node {
	nodeInfo, err := cs.ep.clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		cs.log.Error(err)
	}
	return nodeInfo
}
