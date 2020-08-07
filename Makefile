test:
	./scripts/validate-license.sh
	go test ./cmd/main
	golangci-lint run
build:
	docker build . -t paskalmaksim/envoy-control-plane:dev