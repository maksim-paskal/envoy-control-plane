test:
	./scripts/validate-license.sh
	go test ./cmd/main
	golangci-lint run
testChart:
	helm lint --strict ./chart/envoy-control-plane
	helm template ./chart/envoy-control-plane | kubectl apply --dry-run --validate -f -
build:
	docker build . -t paskalmaksim/envoy-control-plane:dev
k8sConfig:
	kubectl apply -f ./config/
runEnvoy:
	docker-compose down --remove-orphans && docker-compose up
installDev:
	helm delete --purge envoy-control-plane || true
	kubectl delete -n envoy-control-plane -f ./chart/testPods.yaml || true
	kubectl delete ns envoy-control-plane || true
	helm install --namespace envoy-control-plane --name envoy-control-plane ./chart/envoy-control-plane
	kubectl apply -n envoy-control-plane -f ./chart/testPods.yaml
	watch kubectl -n envoy-control-plane get pods
installDevConfig:
	kubectl -n envoy-control-plane apply -f ./chart/envoy-control-plane/templates/envoy-test1-id.yaml
clean:
	helm delete --purge envoy-control-plane || true
	kubectl delete ns envoy-control-plane || true
	kubectl delete -f ./config/ || true
	docker-compose down --remove-orphans