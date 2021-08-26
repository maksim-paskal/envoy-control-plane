KUBECONFIG=$(HOME)/.kube/example-kubeconfig

test:
	./scripts/validate-license.sh
	go fmt ./cmd/...
	go fmt ./pkg/...
	go mod tidy
	./scripts/test-pkg.sh
	golangci-lint run -v
test-release:
	goreleaser release --snapshot --skip-publish --rm-dist
testChart:
	helm lint --strict ./chart/envoy-control-plane
	helm template ./chart/envoy-control-plane | kubectl apply --dry-run=client --validate -f -
build-goreleaser:
	goreleaser build --rm-dist --snapshot
	mv ./dist/envoy-control-plane_linux_amd64/envoy-control-plane envoy-control-plane
	mv ./dist/cli_linux_amd64/cli cli
build:
	make build-goreleaser
	docker build --pull . -t paskalmaksim/envoy-control-plane:dev
	docker build --pull . -f ./envoy/Dockerfile -t paskalmaksim/envoy-docker-image:dev
security-scan:
	trivy fs --ignore-unfixed .
security-check:
	# https://github.com/aquasecurity/trivy
	trivy --ignore-unfixed paskalmaksim/envoy-control-plane:dev
build-envoy:
	make build-goreleaser
	docker-compose build envoy-test1
push:
	docker push paskalmaksim/envoy-control-plane:dev
pushEnvoy:
	docker push paskalmaksim/envoy-docker-image:dev
k8sConfig:
	kubectl apply -f ./chart/envoy-control-plane/templates/testPods.yaml
	kubectl apply -f ./config/
run:
	make k8sConfig
	make build-goreleaser
	docker-compose down --remove-orphans && docker-compose up
runRaceDetection:
	go run -v -race ./cmd/main -log.level=DEBUG -log.pretty -kubeconfig.path=$(KUBECONFIG) -ssl.crt=certs/server.crt -ssl.key=certs/server.key
installDev:
	helm uninstall envoy-control-plane --namespace envoy-control-plane || true
	helm upgrade envoy-control-plane \
  --install \
  --create-namespace \
  --namespace envoy-control-plane \
  ./chart/envoy-control-plane \
  --set withExamples=true \
  --set ingress.enabled=true
	kubectl apply -n envoy-control-plane -f ./chart/envoy-control-plane/templates/testPods.yaml
	watch kubectl -n envoy-control-plane get pods
installDevConfig:
	kubectl -n envoy-control-plane apply -f ./chart/envoy-control-plane/templates/envoy-test1-id.yaml
clean:
	helm uninstall envoy-control-plane --namespace envoy-control-plane || true
	kubectl delete ns envoy-control-plane || true
	kubectl delete -f ./config/ || true
	kubectl delete -f ./chart/envoy-control-plane/templates/testPods.yaml || true
	docker-compose down --remove-orphans
upgrade:
	go get -v -u k8s.io/api@v0.20.9 || true
	go get -v -u k8s.io/apimachinery@v0.20.9
	go get -v -u k8s.io/client-go@v0.20.9
	go mod tidy
heap:
	go tool pprof -http=127.0.0.1:8080 http://localhost:18081/debug/pprof/heap
allocs:
	go tool pprof -http=127.0.0.1:8080 http://localhost:18081/debug/pprof/heap
git-prune-gc:
	curl -sSL https://get.paskal-dev.com/git-prune-gc | sh
sslInit:
	rm -rf ./certs
	mkdir -p ./certs
	openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
	-keyout certs/server.key \
	-out certs/server.crt \
	-subj "/C=GB/ST=London/L=London/O=Global Security/OU=IT Department/CN=*.local"
