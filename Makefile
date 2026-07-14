.PHONY: build build-sidecar test lint docker-build docker-build-sidecar helm-lint

build:
	go build -o bin/manager .

build-sidecar:
	cd sidecar && go build -o ../bin/sidecar ./cmd

test:
	go test ./...
	cd sidecar && go test ./...
	opa test policies/rego -v

lint:
	go vet ./...
	cd sidecar && go vet ./...
	helm lint charts/ai-workload-controller

docker-build:
	docker build -t ai-workload-controller:dev .

docker-build-sidecar:
	docker build -t semantic-guardrail:dev ./sidecar

install-crds:
	kubectl apply -f config/crd/bases/

uninstall-crds:
	kubectl delete -f config/crd/bases/
