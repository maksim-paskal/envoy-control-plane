test:
	./scripts/validate-license.sh
	go test ./cmd/main
	golangci-lint run
run:
	go build -v ./cmd/main
	MY_POD_NAMESPACE=default ./main -namespaced
build:
	docker build . -t paskalmaksim/envoy-control-plane:dev