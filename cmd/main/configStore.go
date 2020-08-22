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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigStore struct {
	config              ConfigType
	ep                  *EndpointsStore
	kubernetesEndpoints sync.Map
	lastEndpoints       []types.Resource
	isTypeInited        bool
}

func newConfigStore(config ConfigType, ep *EndpointsStore) *ConfigStore {
	cs := ConfigStore{
		config:       config,
		ep:           ep,
		isTypeInited: false,
	}

	for _, v := range ep.informer.GetStore().List() {
		pod := v.(*v1.Pod)
		cs.LoadEndpoint(pod)
	}
	cs.saveLastEndpoints()

	cs.Push()
	cs.isTypeInited = true
	return &cs
}

func (cs *ConfigStore) newPod(pod *v1.Pod) {
	// ignore new pods while newConfigStore
	if cs.isTypeInited {
		cs.LoadEndpoint(pod)
		cs.saveLastEndpoints()
	}
}

func (cs *ConfigStore) Push() {
	version := uuid.New().String()
	snap := getConfigSnapshot(version, cs.config, cs.lastEndpoints)

	err := snapshotCache.SetSnapshot(cs.config.Id, snap)
	if err != nil {
		log.Error(err)
	}
	log.Infof("pushed node=%s,version=%s", cs.config.Id, version)
}
func (cs *ConfigStore) LoadEndpoint(pod *v1.Pod) {
	podInfo := cs.podInfo(pod)

	if podInfo.check {
		cs.kubernetesEndpoints.Store(pod.Name, podInfo)
	}
}

func (cs *ConfigStore) saveLastEndpoints() {
	endpoints := yamlToResources(cs.config.Endpoints, api.ClusterLoadAssignment{})

	lbEndpoints := make(map[string][]*endpoint.LocalityLbEndpoints)

	for _, ep := range endpoints {
		fixed := ep.(*api.ClusterLoadAssignment)

		lbEndpoints[fixed.GetClusterName()] = append(lbEndpoints[fixed.GetClusterName()], fixed.GetEndpoints()...)
	}

	cs.kubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
		info := value.(checkPodResult)
		if info.ready {
			nodeLocality := &core.Locality{}

			if *appConfig.ZoneLabels {
				nodeLocality = &core.Locality{
					Region: info.nodeRegion,
					Zone:   info.nodeZone,
				}
			}

			var priority uint32 = 0

			if info.priority > 0 {
				priority = info.priority
			}

			lbEndpoints[info.clusterName] = append(lbEndpoints[info.clusterName], &endpoint.LocalityLbEndpoints{
				Locality: nodeLocality,
				Priority: priority,
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
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
					log.Errorf("clusterName=%s,ip=%s is invalid", clusterName, address)
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
		log.Debugf("new endpoints,node=%s", cs.config.Id)
		// endpoints changes
		cs.Push()
	}
}

type checkPodResult struct {
	check       bool
	clusterName string
	podIP       string
	port        uint32
	ready       bool
	nodeRegion  string
	nodeZone    string
	priority    uint32
}

func (cs *ConfigStore) podInfo(pod *v1.Pod) checkPodResult {
	for _, config := range cs.config.Kubernetes {
		if config.Namespace == pod.Namespace || len(config.Namespace) == 0 {
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
					check:       true,
					clusterName: config.ClusterName,
					podIP:       pod.Status.PodIP,
					ready:       ready,
					port:        config.Port,
					priority:    config.Priority,
				}
				if ready && *appConfig.ZoneLabels {
					nodeInfo := cs.getNode(pod.Spec.NodeName)

					result.nodeRegion = nodeInfo.Labels[*appConfig.NodeRegionLabel]
					result.nodeZone = nodeInfo.Labels[*appConfig.NodeZoneLabel]
				}
				return result
			}
		}
	}
	return checkPodResult{check: false}
}

func (cs *ConfigStore) Stop() {
	log.Info("stop")
}

func (cs *ConfigStore) getNode(nodeName string) *v1.Node {
	nodeInfo, err := cs.ep.clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		log.Error(err)
	}
	return nodeInfo
}
