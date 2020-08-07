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
	"log"
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type EndpointsStore struct {
	clientset *kubernetes.Clientset
	stopCh    chan struct{}
}

func newEndpointsStore(clientset *kubernetes.Clientset, config *map[string]ConfigFile) *EndpointsStore {
	es := EndpointsStore{
		clientset: clientset,
	}

	go func() {
		var factory informers.SharedInformerFactory

		namespace := os.Getenv("MY_POD_NAMESPACE")

		if *namespaced {
			if len(namespace) == 0 {
				log.Panic("no namespace")
			}
			factory = informers.NewSharedInformerFactoryWithOptions(
				es.clientset, 0,
				informers.WithNamespace(namespace),
			)
		} else {
			factory = informers.NewSharedInformerFactoryWithOptions(
				es.clientset, 0,
			)
		}

		informer := factory.Core().V1().Pods().Informer()
		es.stopCh = make(chan struct{})

		informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				es.envoyConfig(pod)
			},
			UpdateFunc: func(oldObj interface{}, newObj interface{}) {
				pod := newObj.(*v1.Pod)
				es.envoyConfig(pod)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				es.envoyConfig(pod)
			},
		})
		informer.Run(es.stopCh)
	}()

	return &es
}

func (es *EndpointsStore) Stop() {
	close(es.stopCh)
}

func (es *EndpointsStore) envoyConfig(pod *v1.Pod) {
	ready := false

	if pod.Status.Phase == v1.PodRunning {
		for _, v := range pod.Status.Conditions {
			if v.Type == v1.PodReady && v.Status == "True" {
				ready = true
			}
		}
	}

	for _, v := range es.searchPod(pod) {
		idx := fmt.Sprintf("%s-%s", v.clusterName, v.podIP)

		if ready {
			configStore[v.nodeId].epStore.Store(idx, v)
		} else {
			configStore[v.nodeId].epStore.Delete(idx)
		}
		configStore[v.nodeId].Push()
	}
}

type checkPodResult struct {
	nodeId      string
	clusterName string
	podIP       string
	port        uint32
}

func (es *EndpointsStore) searchPod(pod *v1.Pod) []checkPodResult {

	var result []checkPodResult

	config := configStore

	for _, v := range config {
		for _, v1 := range v.config.Kubernetes {
			if v1.Namespace == pod.Namespace || len(v1.Namespace) == 0 {
				labelsFound := 0
				for k2, v2 := range pod.Labels {
					if v1.Selector[k2] == v2 {
						labelsFound = labelsFound + 1
					}
				}
				if labelsFound == len(v1.Selector) {
					result = append(result, checkPodResult{
						nodeId:      v.config.Id,
						clusterName: v1.ClusterName,
						podIP:       pod.Status.PodIP,
						port:        v1.Port,
					})
				}
			}
		}
	}

	return result
	/*

		if pod.Namespace == config.Namespace {
			labelsFound := 0
			for k, v := range pod.Labels {
				if config.Selector[k] == v {
					labelsFound = labelsFound + 1
				}
			}
			if labelsFound != len(config.Selector) {
				return
			}
			ready := false

			if pod.Status.Phase == v1.PodRunning {
				for _, v := range pod.Status.Conditions {
					if v.Type == v1.PodReady && v.Status == "True" {
						ready = true
					}
				}
			}

			if ready {
				es.podStore.Store(pod.Name, pod.Status.PodIP)
			} else {
				es.podStore.Delete(pod.Name)
			}

			var newIps []string
			es.podStore.Range(func(key interface{}, value interface{}) bool {
				newIps = append(newIps, value.(string))
				return true
			})
			if !reflect.DeepEqual(es.ips, newIps) {
				es.ips = newIps

				endpoints := make([]types.Resource, len(newIps))

				for k, v := range newIps {
					endpoints[k] = &endpoint.ClusterLoadAssignment{
						ClusterName: config.ClusterName,
						Endpoints: []*endpointv2.LocalityLbEndpoints{{
							LbEndpoints: []*endpointv2.LbEndpoint{{
								HostIdentifier: &endpointv2.LbEndpoint_Endpoint{
									Endpoint: &endpointv2.Endpoint{
										Address: &core.Address{
											Address: &core.Address_SocketAddress{
												SocketAddress: &core.SocketAddress{
													Protocol: core.SocketAddress_TCP,
													Address:  v,
													PortSpecifier: &core.SocketAddress_PortValue{
														PortValue: config.Port,
													},
												},
											},
										},
									},
								},
							}},
						}},
					}
				}
			}
		}
	*/
}
