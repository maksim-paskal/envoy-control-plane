test:
	./scripts/validate-license.sh
	go test ./cmd/main
	golangci-lint run
build:
	docker build . -t paskalmaksim/envoy-control-plane:dev
k8sConfig:
	kubectl apply -f config.k8s/test1-id.yaml
runEnvoy:
	docker-compose down --remove-orphans && docker-compose up