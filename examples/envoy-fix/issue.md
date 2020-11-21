We have our implementation of go-control-plane, it's work great on envoy v1.15.2 - but upgrading to envoy v1.16.0 - got a Caught Segmentation fault on CDS message with aggregated cluster

```
# envoyproxy/envoy-alpine-debug:v1.15.2 - works
# envoyproxy/envoy-alpine-debug:v1.16.0 - not-works
# envoyproxy/envoy-alpine-debug:v1.16.1 - not-works
docker run \
  -it \
  --rm \
  -v $(pwd)/config/:/etc/envoy \
  -p 8000:8000 \
  -p 8001:8001 \
  envoyproxy/envoy-alpine-debug:v1.16.0 \
  --config-path /etc/envoy/envoy.yaml \
  --log-level warn \
  --log-format "%v" \
  --bootstrap-version 3 \
  --service-cluster test \
  --service-node test1-id \
  --service-zone test-zone
```

**envoy.yaml**
```
dynamic_resources:
  cds_config:
    path: /etc/envoy/cds.json
admin:
  access_log_path: "/dev/null"
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 8001
static_resources:
  listeners:
  - name: listener_0
    address:
      socket_address:
        address: 0.0.0.0
        port_value: 8000
    traffic_direction: INBOUND
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: ingress_http
          codec_type: AUTO
          rds:
            route_config_name: test
            config_source:
              path: "/etc/envoy/rds.json"
          http_filters:
          - name: envoy.filters.http.router
```

**cds.json**
```
{
  "versionInfo": "d8e61033-fec9-4565-8ce3-a40d7a565b90",
  "resources": [
    {
      "@type": "type.googleapis.com/envoy.config.cluster.v3.Cluster",
      "name": "local_service2",
      "type": "STRICT_DNS",
      "connectTimeout": "0.250s",
      "loadAssignment": {
        "clusterName": "local_service2",
        "endpoints": [
          {
            "locality": {
              "zone": "us-east-1a"
            },
            "lbEndpoints": [
              {
                "endpoint": {
                  "address": {
                    "socketAddress": {
                      "address": "nginxdemo-a",
                      "portValue": 80
                    }
                  }
                }
              }
            ]
          },
          {
            "locality": {
              "zone": "us-east-1b"
            },
            "lbEndpoints": [
              {
                "endpoint": {
                  "address": {
                    "socketAddress": {
                      "address": "nginxdemo-b",
                      "portValue": 80
                    }
                  }
                }
              }
            ]
          }
        ]
      },
      "healthChecks": [
        {
          "timeout": "1s",
          "interval": "5s",
          "unhealthyThreshold": 3,
          "healthyThreshold": 1,
          "httpHealthCheck": {
            "path": "/ready"
          }
        }
      ],
      "circuitBreakers": {
        "thresholds": [
          {
            "maxConnections": 10,
            "maxPendingRequests": 10,
            "maxRequests": 10
          }
        ]
      }
    },
    {
      "@type": "type.googleapis.com/envoy.config.cluster.v3.Cluster",
      "name": "admin_cluster",
      "type": "STATIC",
      "connectTimeout": "0.250s",
      "loadAssignment": {
        "clusterName": "admin_cluster",
        "endpoints": [
          {
            "lbEndpoints": [
              {
                "endpoint": {
                  "address": {
                    "socketAddress": {
                      "address": "127.0.0.1",
                      "portValue": 18000
                    }
                  }
                }
              }
            ]
          }
        ]
      }
    },
    {
      "@type": "type.googleapis.com/envoy.config.cluster.v3.Cluster",
      "name": "tls-cluster-example",
      "type": "STATIC",
      "connectTimeout": "1s",
      "loadAssignment": {
        "clusterName": "tls-cluster-example",
        "endpoints": [
          {
            "lbEndpoints": [
              {
                "endpoint": {
                  "address": {
                    "socketAddress": {
                      "address": "127.0.0.1",
                      "portValue": 443
                    }
                  }
                }
              }
            ]
          }
        ]
      },
      "healthChecks": [
        {
          "timeout": "1s",
          "interval": "5s",
          "unhealthyThreshold": 3,
          "healthyThreshold": 1,
          "httpHealthCheck": {
            "host": "some.com",
            "path": "/status.php"
          }
        }
      ],
      "transportSocket": {
        "name": "envoy.transport_sockets.tls",
        "typedConfig": {
          "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"
        }
      }
    },
    {
      "@type": "type.googleapis.com/envoy.config.cluster.v3.Cluster",
      "name": "aggregate-cluster",
      "clusterType": {
        "name": "envoy.clusters.aggregate",
        "typedConfig": {
          "@type": "type.googleapis.com/envoy.extensions.clusters.aggregate.v3.ClusterConfig",
          "clusters": [
            "tls-cluster-example",
            "local_service2",
            "admin_cluster"
          ]
        }
      },
      "connectTimeout": "0.250s",
      "lbPolicy": "CLUSTER_PROVIDED"
    }
  ],
  "typeUrl": "type.googleapis.com/envoy.config.cluster.v3.Cluster",
  "nonce": "1"
}
```

**rds.json**
```
{
  "versionInfo": "d8e61033-fec9-4565-8ce3-a40d7a565b90",
  "resources": [
    {
      "@type": "type.googleapis.com/envoy.config.route.v3.RouteConfiguration",
      "name": "test",
      "virtualHosts": [
        {
          "name": "test",
          "domains": [
            "*"
          ],
          "routes": [
            {
              "match": {
                "prefix": "/1"
              },
              "route": {
                "cluster": "local_service1"
              }
            },
            {
              "match": {
                "prefix": "/2"
              },
              "route": {
                "cluster": "local_service2"
              }
            },
            {
              "match": {
                "prefix": "/tls-cluster-example"
              },
              "route": {
                "cluster": "tls-cluster-example"
              }
            },
            {
              "match": {
                "prefix": "/aggregate-cluster"
              },
              "route": {
                "cluster": "aggregate-cluster"
              }
            },
            {
              "match": {
                "prefix": "/"
              },
              "route": {
                "weightedClusters": {
                  "clusters": [
                    {
                      "name": "local_service1",
                      "weight": 50
                    },
                    {
                      "name": "local_service2",
                      "weight": 50
                    }
                  ]
                }
              }
            }
          ]
        }
      ]
    }
  ],
  "typeUrl": "type.googleapis.com/envoy.config.route.v3.RouteConfiguration",
  "nonce": "1"
}
```

**envoy output**
```
Envoy version: 8fb3cb86082b17144a80402f5367ae65f06083bd/1.16.0/Clean/RELEASE/BoringSSL
#0: [0x7ff2f02ae3d0]
#1: Envoy::ThreadLocal::InstanceImpl::runOnAllThreads() [0x557b008d1cd3]
#2: Envoy::ThreadLocal::InstanceImpl::SlotImpl::runOnAllThreads() [0x557b008d1aff]
#3: Envoy::Extensions::Clusters::Aggregate::Cluster::refresh() [0x557affce6339]
#4: Envoy::Extensions::Clusters::Aggregate::Cluster::startPreInit() [0x557affce60cd]
#5: Envoy::Upstream::ClusterImplBase::initialize() [0x557b00a85585]
#6: Envoy::Upstream::ClusterManagerInitHelper::initializeSecondaryClusters() [0x557b00937feb]
#7: Envoy::Upstream::ClusterManagerInitHelper::maybeFinishInitialize() [0x557b009377dc]
#8: Envoy::Upstream::ClusterManagerInitHelper::removeCluster() [0x557b00936afe]
#9: Envoy::Upstream::ClusterImplBase::finishInitialization() [0x557b00a85ad7]
#10: Envoy::Upstream::ClusterImplBase::onInitDone() [0x557b00a85933]
#11: Envoy::Init::WatcherHandleImpl::ready() [0x557b00cbf74b]
#12: Envoy::Init::ManagerImpl::initialize() [0x557b00cbc44b]
#13: Envoy::Upstream::ClusterImplBase::onPreInitComplete() [0x557b00a8580d]
#14: std::__1::__function::__func<>::operator()() [0x557b00aaa4e9]
#15: Envoy::Network::DnsResolverImpl::PendingResolution::onAresGetAddrInfoCallback() [0x557b009000d6]
#16: end_hquery [0x557b006e71d5]
#17: next_lookup [0x557b006e70b6]
#18: qcallback [0x557b006ea773]
#19: end_query [0x557b006e9b15]
#20: process_answer [0x557b006ea34b]
#21: processfds [0x557b006e8d50]
#22: std::__1::__function::__func<>::operator()() [0x557b009028a5]
#23: Envoy::Event::FileEventImpl::assignEvents()::$_1::__invoke() [0x557b008fc366]
#24: event_process_active_single_queue [0x557b00d34fe8]
#25: event_base_loop [0x557b00d339be]
#26: Envoy::Server::InstanceImpl::run() [0x557b008df79c]
#27: Envoy::MainCommonBase::run() [0x557affc3d008]
#28: Envoy::MainCommon::main() [0x557affc3d807]
#29: main [0x557affc3bbdc]
#30: __libc_start_main [0x7ff2f00fbc8d]
```