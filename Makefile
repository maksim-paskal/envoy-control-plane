test:
	./scripts/validate-license.sh
	go fmt ./cmd/main
	go fmt ./cmd/cli
	go mod tidy
	go test ./cmd/main
	go test ./cmd/cli
	golangci-lint run --allow-parallel-runners -v --enable-all --disable nestif,gochecknoglobals,funlen,gocognit --fix
testChart:
	helm lint --strict ./chart/envoy-control-plane
	helm template ./chart/envoy-control-plane | kubectl apply --dry-run --validate -f -
build:
	docker build . -t paskalmaksim/envoy-control-plane:dev-v3
buildEnvoy:
	docker build ./envoy -t paskalmaksim/envoy-docker-image:dev-v3
build-cli:
	./scripts/build-cli.sh
push:
	docker push paskalmaksim/envoy-control-plane:dev-v3
pushEnvoy:
	docker push paskalmaksim/envoy-docker-image:dev-v3
k8sConfig:
	kubectl apply -f ./chart/testPods.yaml
	kubectl apply -f ./config/
runEnvoy:
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