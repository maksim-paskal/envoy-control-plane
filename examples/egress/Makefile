KUBECONFIG=$(HOME)/.kube/dev

deploy:
	make clean
	kubectl create cm envoy-config \
	--from-file=envoy.yaml \
	--from-file=certs/server.key \
	--from-file=certs/server.crt
	kubectl apply -f test.yaml
clean:
	kubectl delete cm envoy-config || true
	kubectl delete -f test.yaml || true
run:
	docker-compose down --remove-orphans; docker-compose up
genCerts:
	rm -rf certs
	mkdir certs
	go run ../../cmd/gencerts -dns.names=\
	get.paskal-dev.com,\
	google.com,\
	www.recaptcha.net

	openssl x509 -in ./certs/server.crt -text -noout