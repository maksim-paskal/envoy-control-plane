KUBECONFIG=$(HOME)/.kube/kurento-stage
initialPodCount=10
gitTag=$(shell git rev-parse --abbrev-ref HEAD)

test:
	./scripts/validate-license.sh
	go fmt ./cmd/... ./pkg/...
	go vet ./cmd/... ./pkg/...
	go mod tidy
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run -v
	./scripts/test-pkg.sh
coverage:
	go tool cover -html=coverage.out
testChart:
	ct lint --target-branch main --all
	helm lint --strict ./charts/envoy-control-plane
	helm template ./charts/envoy-control-plane | kubectl apply --dry-run=client --validate -f -

	helm dep up ./charts/envoy-ratelimit-service
	helm lint --strict ./charts/envoy-ratelimit-service
	helm template ./charts/envoy-ratelimit-service | kubectl apply --dry-run=client --validate -f -

	rm -rf ./examples/test-deploy/charts
	helm dep up ./examples/test-deploy
	ct lint --charts ./examples/test-deploy
	helm template ./examples/test-deploy | kubectl apply --dry-run=client --validate -f -
build-goreleaser:
	git tag -d `git tag -l "envoy-*"`
	git tag -d `git tag -l "helm-chart-*"`
	go run github.com/goreleaser/goreleaser@latest build --clean --snapshot
	mv ./dist/envoy-control-plane_linux_amd64_v1/envoy-control-plane envoy-control-plane
	mv ./dist/cli_linux_amd64_v1/cli cli
build:
	make build-goreleaser
	docker build --pull --push --platform=linux/amd64 . -t paskalmaksim/envoy-control-plane:$(gitTag)
	docker build --pull --push --platform=linux/amd64 . -t paskalmaksim/envoy-docker-image:$(gitTag) -f ./envoy/Dockerfile
promote-to-beta:
	git tag -d `git tag -l "envoy-*"`
	git tag -d `git tag -l "helm-chart-*"`
	go run github.com/goreleaser/goreleaser@latest release --clean --snapshot

	docker push paskalmaksim/envoy-docker-image:beta-arm64
	docker push paskalmaksim/envoy-docker-image:beta-amd64
	docker push paskalmaksim/envoy-control-plane:beta-arm64
	docker push paskalmaksim/envoy-control-plane:beta-amd64

	docker manifest create --amend paskalmaksim/envoy-docker-image:beta \
	paskalmaksim/envoy-docker-image:beta-arm64 \
	paskalmaksim/envoy-docker-image:beta-amd64
	docker manifest push --purge paskalmaksim/envoy-docker-image:beta

	docker manifest create --amend paskalmaksim/envoy-control-plane:beta \
	paskalmaksim/envoy-control-plane:beta-arm64 \
	paskalmaksim/envoy-control-plane:beta-amd64
	docker manifest push --purge paskalmaksim/envoy-control-plane:beta
build-composer:
	docker-compose build --pull --parallel
security-scan:
	trivy fs --ignore-unfixed .
security-check:
	# https://github.com/aquasecurity/trivy
	trivy --ignore-unfixed paskalmaksim/envoy-control-plane:$(gitTag)
push:
	docker push paskalmaksim/envoy-control-plane:$(gitTag)
	docker push paskalmaksim/envoy-docker-image:$(gitTag)
k8sConfig:
	kubectl -n default apply -f ./examples/test-deploy/templates/testPods.yaml
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
	-namespace=default \
	-kubeconfig.path=$(KUBECONFIG) \
	-web.adminUser=admin \
	-web.adminPassword=admin \
	-ssl.crt=certs/CA.crt \
	-ssl.key=certs/CA.key \
	-leaderElection=false \
	-grpc.address=127.0.0.1:18080 \
	-web.https.address=127.0.0.1:18081 \
	-web.http.address=127.0.0.1:18082
runCli:
	go run ./cmd/cli -debug -namespace=1 -pod=2 \
	-tls.CA=certs/CA.crt \
	-tls.Crt=certs/envoy.crt \
	-tls.Key=certs/envoy.key

installDev:
	rm -rf ./examples/test-deploy/charts
	helm dep up ./examples/test-deploy

	helm uninstall test-deploy --namespace envoy-control-plane || true
	helm upgrade test-deploy \
	--install \
	--create-namespace \
	--namespace envoy-control-plane \
	./examples/test-deploy \
	--set image.repository=paskalmaksim/envoy-docker-image \
	--set image.tag=$(gitTag) \
	--set image.pullPolicy=Always \
	--set envoy-control-plane.image.repository=paskalmaksim/envoy-control-plane \
	--set envoy-control-plane.image.tag=$(gitTag) \
	--set envoy-control-plane.image.pullPolicy=Always \
	--set-file envoy-control-plane.certificates.caKey=./certs/CA.key \
	--set-file envoy-control-plane.certificates.caCrt=./certs/CA.crt \
	--set-file envoy-control-plane.certificates.envoyKey=./certs/envoy.key \
	--set-file envoy-control-plane.certificates.envoyCrt=./certs/envoy.crt

	watch kubectl -n envoy-control-plane get pods
installDevConfig:
	kubectl -n envoy-control-plane apply -f ./charts/envoy-control-plane/templates/envoy-test1-id.yaml
clean:
	helm uninstall test-deploy --namespace envoy-control-plane || true
	kubectl delete ns envoy-control-plane || true
	kubectl -n default delete cm -lapp=envoy-control-plane || true
	kubectl -n default delete -f ./config/ || true
	kubectl -n default delete -f ./examples/test-deploy/templates/testPods.yaml || true
	docker-compose down --remove-orphans
upgrade:
	go get -v -u k8s.io/api@v0.21.10 || true
	go get -v -u k8s.io/apimachinery@v0.21.10
	go get -v -u k8s.io/client-go@v0.21.10
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
	openssl rsa -in ./certs/server.key -check -noout
	openssl verify -CAfile ./certs/CA.crt ./certs/server.crt
	openssl verify -CAfile ./certs/CA.crt ./certs/envoy.crt

	openssl x509 -in ./certs/server.crt -text
	openssl x509 -in ./certs/envoy.crt -text

	openssl x509 -pubkey -in ./certs/CA.crt -noout | openssl md5
	openssl pkey -pubout -in ./certs/CA.key | openssl md5

	openssl x509 -pubkey -in ./certs/server.crt -noout | openssl md5
	openssl pkey -pubout -in ./certs/server.key | openssl md5
sslTestClient:
	curl -v --cacert ./certs/CA.crt --resolve "test2-id:8001:127.0.0.1" --key ./certs/server.key --cert ./certs/server.crt https://test2-id:8001
	curl -v --cacert ./certs/CA.crt --resolve "test3-id:8002:127.0.0.1" --key ./certs/server.key --cert ./certs/server.crt https://test3-id:8002
sslTestControlPlane:
	curl -vk --http2 --cacert ./certs/CA.crt --resolve "envoy-control-plane:18080:127.0.0.1" --key ./certs/server.key --cert ./certs/server.crt https://envoy-control-plane:18080

.PHONY: e2e
e2e:
	make build clean k8sConfig gitTag=e2e-test
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

	make clean
scan:
	@trivy image \
	--ignore-unfixed --no-progress --severity HIGH,CRITICAL \
	paskalmaksim/envoy-control-plane:$(gitTag)
	@trivy image \
	--ignore-unfixed --no-progress --severity HIGH,CRITICAL \
	paskalmaksim/envoy-docker-image:$(gitTag)