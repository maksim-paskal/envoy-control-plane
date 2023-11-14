# Lightweight Envoy control plane

## Motivation

Popular Istio or Linkerd is very complex to do something simple with kubernetes service discovery and use all Envoy benefits (circuit_breakers, retry_policy, health_checks or zone aware routing)

### Generate TLS certificates for secure envoy to envoy-control-plane connection

envoy-control-plane always start in secured mode, it use mTLS for securing traffic between envoy-control-plane and envoy. To start envoy-control-plane you need to create CA certificates that will be used in mTLS communication, also envoy need client certificate to connect envoy-control-plane

simple way to create CA certificate and envoy certificate

```bash
make sslInit
```

files `./certs/envoy.key`, `./certs/envoy.crt` and `./certs/CA.crt` must be used in envoy to establish secure connection to envoy-control-plane, files `./certs/CA.key` and `./certs/CA.crt` must be used in envoy-control-plane

### Run control plane in your application namespace

```bash
helm repo add maksim-paskal-envoy-control-plane https://maksim-paskal.github.io/envoy-control-plane/
helm repo update

helm upgrade envoy-control-plane \
  --install \
  --create-namespace \
  --namespace envoy-control-plane \
  maksim-paskal-envoy-control-plane/envoy-control-plane \
  --set withExamples=true \
  --set ingress.enabled=true \
  --set-file certificates.caKey=./certs/CA.key \
  --set-file certificates.caCrt=./certs/CA.crt \
  --set-file certificates.envoyKey=./certs/envoy.key \
  --set-file certificates.envoyCrt=./certs/envoy.crt

# get NAME of envoy POD, must be in Ready state
kubectl -n envoy-control-plane get pods -lapp=envoy -o wide

# port forward to envoy pod, open browser http://127.0.0.1:8000, success result `Hello World`
kubectl -n envoy-control-plane port-forward <envoy-pod-name> 8000

# port forward to envoy pod, open browser https://127.0.0.1:18000, envoy administration interface
# https://www.envoyproxy.io/docs/envoy/latest/operations/admin
kubectl -n envoy-control-plane port-forward <envoy-pod-name> 18000

# to uninstall
helm uninstall envoy-control-plane -n envoy-control-plane
kubectl delete ns envoy-control-plane
```

### To integrate your POD to envoy-control-plane

To integrate your application to envoy-control-plane - you need to add sidecar container, and certificates that will be used to secure connection between envoy and envoy-control-plane

```yaml
volumes:
- name: certs
  configMap:
    name: envoy-certs # ConfigMap that store CA.crt, envoy.crt, envoy.key
containers:
- name: envoy
  lifecycle:
    preStop:
      exec:
        # gracefully drain all connection and shutdown
        command:
        - cli
        - -drainEnvoy
        - -timeout=10s
  image: paskalmaksim/envoy-docker-image:latest
  imagePullPolicy: Always
  args:
  - /bin/sh
  - -c
  - |
    /usr/local/bin/envoy \
    --config-path /etc/envoy/envoy.yaml \
    --log-level warn \
    --service-cluster test \
    --service-node test1-id
  resources:
    requests:
      cpu: 10m
      memory: 100Mi
  env:
  - name: MY_POD_NAMESPACE
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.namespace
  # this will be send all traces to jaeger agent (daemonset)
  - name: OTLP_COLLECTOR_HOST
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: status.hostIP
  # jaeger traces service name
  - name: ENVOY_SERVICE_NAME
    value: "test-envoy-service"
  readinessProbe:
    httpGet:
      path: /ready
      port: 18001
    initialDelaySeconds: 3
    periodSeconds: 5
  livenessProbe:
    httpGet:
      path: /server_info
      port: 18001
    initialDelaySeconds: 60
    periodSeconds: 10

  ports:
  - containerPort: 8000   # application proxy
  - containerPort: 18000  # envoy admin
```

### Configurate your envoy sidecars with simple ConfigMap

Sample configuration [here](chart/envoy-control-plane/templates/envoy-test1-id.yaml)

### Prometheus metrics

envoy-control-plane expose metrics on `/api/metrics` endpoint in web interface - for static configuration use this scrape config:

```yaml
scrape_configs:
- job_name: envoy-control-plane
  scrape_interval: 1s
  metrics_path: /api/metrics
  static_configs:
  - targets:
    - <envoy-control-plane-ip>:18081
```

or just add pod annotation

```yaml
annotations:
  prometheus.io/path: '/api/metrics'
  prometheus.io/scrape: 'true'
  prometheus.io/port: '18081'
```
