# Lightweight Envoy control plane
## Motivation
Popular Istio or Linkerd is very complex to do something simple with kubernetes service discovery and use all Envoy benefits (circuit_breakers, retry_policy, health_checks or zone aware routing)

## Simple configuration
### Run control plane in your application namespace
```bash
git clone git@github.com:maksim-paskal/envoy-control-plane.git

helm upgrade envoy-control-plane \
  --install \
  --create-namespace \
  --namespace envoy-control-plane \
  ./chart/envoy-control-plane \
  --set withExamples=true \
  --set ingress.enabled=true

# test
curl -k -H "Host: test.dev.com" https://<IngressAddress>/2


# to uninstall
helm uninstall envoy-control-plane -n envoy-control-plane
kubectl delete ns envoy-control-plane
```

### Add sidecar to your pod
```yaml
- name: envoy
  image: paskalmaksim/envoy-docker-image:latest
  imagePullPolicy: Always
  args:
  - /bin/sh
  - -c
  - |
    /usr/local/bin/envoy \
    --config-path /etc/envoy/envoy.yaml \
    --log-level warn \
    --bootstrap-version 3 \
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
  - name: JAEGER_AGENT_HOST
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
      port: 18000
    initialDelaySeconds: 3
    periodSeconds: 5
  livenessProbe:
    httpGet:
      path: /server_info
      port: 18000
    initialDelaySeconds: 60
    periodSeconds: 10

  ports:
  - containerPort: 8000   # application proxy
  - containerPort: 18000  # envoy admin
```
### Configurate your envoy sidecars with with ConfigMap
sample configuration here https://github.com/maksim-paskal/envoy-control-plane/blob/main/chart/envoy-control-plane/templates/envoy-test1-id.yaml

### Prometheus metrics
envoy-control-plane expose metrics on `/api/metrics` endpoint in web interface - for static configuration use this scrape config:
```
scrape_configs:
- job_name: envoy-control-plane
  scrape_interval: 1s
  metrics_path: /api/metrics
  static_configs:
  - targets:
    - <envoy-control-plane-ip>:18081
```
or just add pod annotation
```
annotations:
  prometheus.io/path: '/api/metrics'
  prometheus.io/scrape: 'true'
  prometheus.io/port: '18081'
```