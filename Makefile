test:
	./scripts/validate-license.sh
	go fmt ./cmd/main
	go fmt ./cmd/cli
	go mod tidy
	go test -race ./cmd/main
	go test -race ./cmd/cli
	golangci-lint run --allow-parallel-runners -v --enable-all --disable nestif,gochecknoglobals,funlen,gocognit,exhaustivestruct,cyclop --fix
testChart:
	helm lint --strict ./chart/envoy-control-plane
	helm template ./chart/envoy-control-plane | kubectl apply --dry-run --validate -f -
build:
	docker build . -t paskalmaksim/envoy-control-plane:dev
buildEnvoy:
	docker build ./envoy -t paskalmaksim/envoy-docker-image:dev
build-cli:
	./scripts/build-cli.sh
push:
	docker push paskalmaksim/envoy-control-plane:dev
pushEnvoy:
	docker push paskalmaksim/envoy-docker-image:dev
k8sConfig:
	kubectl apply -f ./chart/testPods.yaml
	kubectl apply -f ./config/
run:
	@./scripts/build-main.sh
	docker-compose down --remove-orphans && docker-compose up
installDev:
	helm delete --purge envoy-control-plane || true
	helm install --namespace envoy-control-plane --name envoy-control-plane ./chart/envoy-control-plane
	kubectl apply -n envoy-control-plane -f ./chart/testPods.yaml
	kubectl apply -n envoy-control-plane -f ./chart/ingress.yaml
	watch kubectl -n envoy-control-plane get pods
installDevConfig:
	kubectl -n envoy-control-plane apply -f ./chart/envoy-control-plane/templates/envoy-test1-id.yaml
clean:
	helm delete --purge envoy-control-plane || true
	kubectl delete ns envoy-control-plane || true
	kubectl delete -f ./config/ || true
	kubectl delete -f ./chart/testPods.yaml || true
	docker-compose down --remove-orphans
upgrade:
	go get -v -u all
	# downgrade to v0.18.14
	go get -v -u k8s.io/api@v0.18.14 || true
	go get -v -u k8s.io/apimachinery@v0.18.14
	go get -v -u k8s.io/client-go@v0.18.14
	# downgrade for k8s.io/client-go@v0.18.14
	go get -v -u github.com/googleapis/gnostic@v0.1.0
	go mod tidy
heap:
	go tool pprof -http=127.0.0.1:8080 http://localhost:18081/debug/pprof/heap
allocs:
	go tool pprof -http=127.0.0.1:8080 http://localhost:18081/debug/pprof/heap