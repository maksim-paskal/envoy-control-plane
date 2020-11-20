*Description*:
We have our implementation of go-control-plane, we use latest v3 API, it's work on envoy v1.15.2 - but upgrading envoy to v1.16+ we got a Caught Segmentation fault on CDS message with aggregated cluster

```
docker run \
  -it \
  --rm \
  -v $(pwd)/config/:/etc/envoy \
  -p 8000:8000 \
  -p 8001:8001 \
  envoyproxy/envoy-debug-dev:6e08670ac7ff459ef8058427663759acab980175 \
  --config-path /etc/envoy/envoy.yaml \
  --log-level warning \
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
  clusters:
  - name: admin_cluster
    connect_timeout: 0.25s
    lb_policy: ROUND_ROBIN
    type: STATIC
    load_assignment:
      cluster_name: admin_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 18000
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
          route_config:
            name: route
            virtual_hosts:
            - name: backend
              domains:
              - "*"
              routes:
              - match:
                  prefix: /
                route:
                  cluster: admin_cluster
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

**envoy output**
```
Caught Segmentation fault, suspect faulting address 0x0
Backtrace (use tools/stack_decode.py to get line numbers):
Envoy version: 6e08670ac7ff459ef8058427663759acab980175/1.17.0-dev/Clean/RELEASE/BoringSSL
#0: __restore_rt [0x7f7ab3750980]
#1: std::__1::__function::__func<>::operator()() [0x563daaf9fb93]
#2: std::__1::__function::__func<>::operator()() [0x563dac69434c]
#3: Envoy::ThreadLocal::InstanceImpl::runOnAllThreads() [0x563dac693143]
#4: Envoy::ThreadLocal::InstanceImpl::SlotImpl::runOnAllThreads() [0x563dac69300b]
#5: Envoy::ThreadLocal::TypedSlot<>::runOnAllThreads() [0x563daaf9b865]
#6: Envoy::Extensions::Clusters::Aggregate::Cluster::refresh() [0x563daaf9b675]
#7: Envoy::Extensions::Clusters::Aggregate::Cluster::startPreInit() [0x563daaf9b40d]
#8: Envoy::Upstream::ClusterImplBase::initialize() [0x563dac84d865]
#9: Envoy::Upstream::ClusterManagerInitHelper::initializeSecondaryClusters() [0x563dac700354]
#10: Envoy::Upstream::ClusterManagerInitHelper::maybeFinishInitialize() [0x563dac6ffb2c]
#11: Envoy::Upstream::ClusterManagerInitHelper::removeCluster() [0x563dac6fee49]
#12: Envoy::Upstream::ClusterImplBase::finishInitialization() [0x563dac84ddb7]
#13: Envoy::Upstream::ClusterImplBase::onInitDone() [0x563dac84dc13]
#14: Envoy::Init::WatcherHandleImpl::ready() [0x563daca705ab]
#15: Envoy::Init::ManagerImpl::initialize() [0x563daca6d28b]
#16: Envoy::Upstream::ClusterImplBase::onPreInitComplete() [0x563dac84daed]
#17: std::__1::__function::__func<>::operator()() [0x563dac872989]
#18: Envoy::Network::DnsResolverImpl::PendingResolution::onAresGetAddrInfoCallback() [0x563dac6c5226]
#19: end_hquery [0x563daba73ae5]
#20: next_lookup [0x563daba7396a]
#21: qcallback [0x563daba77223]
#22: end_query [0x563daba76405]
#23: process_answer [0x563daba76dfb]
#24: processfds [0x563daba75640]
#25: std::__1::__function::__func<>::operator()() [0x563dac6c79c5]
#26: std::__1::__function::__func<>::operator()() [0x563dac6c0731]
#27: Envoy::Event::FileEventImpl::assignEvents()::$_1::__invoke() [0x563dac6c14cc]
#28: event_process_active_single_queue [0x563dacae9698]
#29: event_base_loop [0x563dacae806e]
#30: Envoy::Server::InstanceImpl::run() [0x563dac6a106f]
#31: Envoy::MainCommonBase::run() [0x563daaee9888]
#32: Envoy::MainCommon::main() [0x563daaeea087]
#33: main [0x563daaee845c]
#34: __libc_start_main [0x7f7ab336ebf7]
```