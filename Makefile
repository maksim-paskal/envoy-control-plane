KUBECONFIG=$(HOME)/.kube/example-kubeconfig
initialPodCount=10

test:
	./scripts/validate-license.sh
	go fmt ./cmd/... ./pkg/...
	go vet ./cmd/... ./pkg/...
	go mod tidy
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run -v
	./scripts/test-pkg.sh
coverage:
	go tool cover -html=coverage.out
test-release:
	git tag -d `git tag -l "helm-chart-*"`
	go run github.com/goreleaser/goreleaser@latest release --snapshot --skip-publish --rm-dist
testChart:
	helm lint --strict ./charts/envoy-control-plane
	helm template ./charts/envoy-control-plane | kubectl apply --dry-run=client --validate -f -
build-goreleaser:
	git tag -d `git tag -l "helm-chart-*"`
	go run github.com/goreleaser/goreleaser@latest build --rm-dist --snapshot
	mv ./dist/envoy-control-plane_linux_amd64/envoy-control-plane envoy-control-plane
	mv ./dist/cli_linux_amd64/cli cli
build:
	make build-goreleaser
	docker build --pull . -t paskalmaksim/envoy-control-plane:dev
	docker build --pull . -f ./envoy/Dockerfile -t paskalmaksim/envoy-docker-image:dev
	docker-compose build --pull --parallel
security-scan:
	trivy fs --ignore-unfixed .
security-check:
	# https://github.com/aquasecurity/trivy
	trivy --ignore-unfixed paskalmaksim/envoy-control-plane:dev
push:
	docker push paskalmaksim/envoy-control-plane:dev
	docker push paskalmaksim/envoy-docker-image:dev
k8sConfig:
	kubectl -n default apply -f ./charts/envoy-control-plane/templates/testPods.yaml
	kubectl -n default apply -f ./config/
run:
	cp ${KUBECONFIG} kubeconfig
	kubectl -n default delete cm -lapp=envoy-control-plane || true
	make k8sConfig
	make build-goreleaser
	docker-compose down --remove-orphans && docker-compose up
runRaceDetection:
	go run -v -race ./cmd/main \
	-log.level=DEBUG \
	-log.pretty \
	-kubeconfig.path=$(KUBECONFIG) \
	-web.adminUser=admin \
	-web.adminPassword=admin \
	-ssl.crt=certs/CA.crt \
	-ssl.key=certs/CA.key
runCli:
	go run ./cmd/cli -debug -namespace=1 -pod=2 \
	-tls.CA=certs/CA.crt \
	-tls.Crt=certs/envoy.crt \
	-tls.Key=certs/envoy.key

installDev:
	helm uninstall envoy-control-plane --namespace envoy-control-plane || true
	helm upgrade envoy-control-plane \
	--install \
	--create-namespace \
	--namespace envoy-control-plane \
	./charts/envoy-control-plane \
	--set withExamples=true \
	--set ingress.enabled=true \
	--set registry.image=paskalmaksim/envoy-control-plane:dev \
	--set envoy.registry.image=paskalmaksim/envoy-docker-image:dev \
	--set-file certificates.caKey=./certs/CA.key \
	--set-file certificates.caCrt=./certs/CA.crt \
	--set-file certificates.envoyKey=./certs/envoy.key \
	--set-file certificates.envoyCrt=./certs/envoy.crt

	kubectl apply -n envoy-control-plane -f ./charts/envoy-control-plane/templates/testPods.yaml
	watch kubectl -n envoy-control-plane get pods
installDevConfig:
	kubectl -n envoy-control-plane apply -f ./charts/envoy-control-plane/templates/envoy-test1-id.yaml
clean:
	helm uninstall envoy-control-plane --namespace envoy-control-plane || true
	kubectl delete ns envoy-control-plane || true
	kubectl -n default delete cm -lapp=envoy-control-plane || true
	kubectl -n default delete -f ./config/ || true
	kubectl -n default delete -f ./charts/envoy-control-plane/templates/testPods.yaml || true
	docker-compose down --remove-orphans
upgrade:
	go get -v -u k8s.io/api@v0.20.13 || true
	go get -v -u k8s.io/apimachinery@v0.20.13
	go get -v -u k8s.io/client-go@v0.20.13
	go mod tidy
heap:
	go tool pprof -http=127.0.0.1:8080 https+insecure://localhost:18081/debug/pprof/heap
allocs:
	go tool pprof -http=127.0.0.1:8080 https+insecure://localhost:18081/debug/pprof/heap
git-prune-gc:
	curl -sSL https://get.paskal-dev.com/git-prune-gc | sh
sslInit:
	rm -rf ./certs/
	mkdir -p ./certs/

	go run ./cmd/gencerts -cert.path=certs
sslTest:
	openssl rsa -in ./certs/CA.key -check -noout
	openssl rsa -in ./certs/test.key -check -noout
	openssl verify -CAfile ./certs/CA.crt ./certs/test.crt
	openssl verify -CAfile ./certs/CA.crt ./certs/envoy.crt

	openssl x509 -in ./certs/test.crt -text
	openssl x509 -in ./certs/envoy.crt -text

	openssl x509 -pubkey -in ./certs/CA.crt -noout | openssl md5
	openssl pkey -pubout -in ./certs/CA.key | openssl md5

	openssl x509 -pubkey -in ./certs/test.crt -noout | openssl md5
	openssl pkey -pubout -in ./certs/test.key | openssl md5
sslTestClient:
	curl -v --cacert ./certs/CA.crt --resolve "test2-id:8001:127.0.0.1" --key ./certs/test.key --cert ./certs/test.crt https://test2-id:8001
	curl -v --cacert ./certs/CA.crt --resolve "test3-id:8002:127.0.0.1" --key ./certs/test.key --cert ./certs/test.crt https://test3-id:8002
sslTestControlPlane:
	curl -vk --http2 --cacert ./certs/CA.crt --resolve "envoy-control-plane:18080:127.0.0.1" --key ./certs/test.key --cert ./certs/test.crt https://envoy-control-plane:18080
test-e2e:
	make clean k8sConfig
	kubectl scale deploy test-001 test-002 --replicas=${initialPodCount}
	kubectl wait --for=condition=available deployment --all --timeout=600s
	kubectl wait --for=condition=Ready pods --all --timeout=600s

	go test -v -race ./e2e \
	-initialPodCount=${initialPodCount} \
	-kubeconfig.path=$(KUBECONFIG) \
	-config.drainPeriod=1s \
	-endpoint.checkPeriod=1s \
	-ssl.rotation=1s \
	-log.level=INFO \
	-log.pretty