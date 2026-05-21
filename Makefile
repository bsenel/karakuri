.PHONY: build test run-server tidy \
        web-install web-dev web-build web-typecheck \
        docker-build docker-up docker-down docker-logs \
        helm-up helm-down helm-logs helm-template \
        minikube-up minikube-down minikube-logs \
        k3s-up k3s-down k3s-logs \
        argocd-install argocd-up argocd-down argocd-open argocd-logs \
        _secret _image-load-minikube _image-load-k3s \
        _helm-install _helm-install-k3s _helm-uninstall

# ── Shared constants ─────────────────────────────────────────────────────────
IMAGE      := karakuri:latest
NAMESPACE  := karakuri
RELEASE    := karakuri
CHART      := deploy
K3S_VALUES := $(CHART)/values-k3s.yaml

# ── Local binary ─────────────────────────────────────────────────────────────

build:
	go build -o bin/server ./cmd/server/
	go build -o bin/krk ./cmd/krk/

test:
	go test ./... -count=1

# ── Web (React SPA in web/) ──────────────────────────────────────────────────
# Requires Node 18+. `web-build` populates web/dist which the Go binary embeds
# at build time, so the standard `make build` will pick up a fresh UI when run
# after `make web-build`.

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

web-typecheck:
	cd web && npm run typecheck

run-server:
	./bin/server

tidy:
	go mod tidy

# ── Internal primitives (used by multiple variants) ──────────────────────────

_secret:
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret generic karakuri-secrets \
	  --from-literal=ANTHROPIC_API_KEY=$${ANTHROPIC_API_KEY} \
	  --from-literal=KARAKURI_AUTH_TOKEN=$${KARAKURI_AUTH_TOKEN:-""} \
	  -n $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -

_image-load-minikube:
	eval $$(minikube docker-env) && docker build -t $(IMAGE) .

_image-load-k3s:
	docker build -t $(IMAGE) .
	docker save $(IMAGE) | sudo k3s ctr images import -

_helm-install:
	helm upgrade --install $(RELEASE) $(CHART) -n $(NAMESPACE) --create-namespace

_helm-install-k3s:
	helm upgrade --install $(RELEASE) $(CHART) -n $(NAMESPACE) --create-namespace -f $(K3S_VALUES)

_helm-uninstall:
	helm uninstall $(RELEASE) -n $(NAMESPACE) || true

# ── Variant A: Docker Compose ────────────────────────────────────────────────

docker-build:
	docker build -t $(IMAGE) .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f karakuri

# ── Variant B: Helm direct (image must already be loadable in the cluster) ───

helm-up: _secret _helm-install

helm-down: _helm-uninstall

helm-logs:
	kubectl logs -f deploy/$(RELEASE) -n $(NAMESPACE)

helm-template:
	helm template $(RELEASE) $(CHART) -n $(NAMESPACE)

# ── Variant C: Minikube ──────────────────────────────────────────────────────

minikube-up: _image-load-minikube _secret _helm-install
	kubectl rollout status deploy/$(RELEASE) -n $(NAMESPACE)
	minikube service $(RELEASE) -n $(NAMESPACE) --url

minikube-down: _helm-uninstall
	minikube stop

minikube-logs: helm-logs

# ── Variant D: k3s ───────────────────────────────────────────────────────────

k3s-up: _image-load-k3s _secret _helm-install-k3s
	kubectl rollout status deploy/$(RELEASE) -n $(NAMESPACE)
	@echo "Run: kubectl port-forward svc/$(RELEASE) 8080:8080 -n $(NAMESPACE)"

k3s-down: _helm-uninstall

k3s-logs: helm-logs

# ── Variant E: ArgoCD (GitOps) ───────────────────────────────────────────────

argocd-install:
	kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	kubectl wait --for=condition=available deploy/argocd-server -n argocd --timeout=120s

argocd-up: argocd-install _secret
	kubectl apply -f deploy/argocd/application.yaml
	@echo "Run: kubectl port-forward svc/argocd-server 8443:443 -n argocd"

argocd-down:
	kubectl delete -f deploy/argocd/application.yaml --ignore-not-found
	$(MAKE) _helm-uninstall

argocd-open:
	@echo "ArgoCD admin password:"
	@kubectl get secret argocd-initial-admin-secret -n argocd \
	  -o jsonpath="{.data.password}" | base64 -d && echo
	@echo "UI: https://localhost:8443"

argocd-logs: helm-logs
