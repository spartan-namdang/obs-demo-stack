.PHONY: all prereqs cluster build infra deploy dashboards forward clean help

# Default target
all: prereqs cluster build infra deploy dashboards
	@echo "✅ Setup Complete!"
	@echo "Run 'make forward-argocd' to access ArgoCD."
	@echo "Wait for all ArgoCD apps to sync, then run 'make forward-grafana' to access Grafana."

help:
	@echo "Available commands:"
	@echo "  make all        - 🚀 Run the entire setup from scratch (One-Click)"
	@echo "  make cluster    - Start Minikube"
	@echo "  make build      - Build Docker images inside Minikube"
	@echo "  make infra      - Provision ArgoCD via Terraform"
	@echo "  make deploy     - Apply K8s App-of-Apps manifests"
	@echo "  make forward    - Port-forward Grafana (localhost:3000)"
	@echo "  make clean      - 💥 Destroy everything (Minikube & Terraform)"

prereqs:
	@echo "Checking prerequisites..."
	@which minikube > /dev/null || (echo "❌ minikube not found" && exit 1)
	@which terraform > /dev/null || (echo "❌ terraform not found" && exit 1)
	@which kubectl > /dev/null || (echo "❌ kubectl not found" && exit 1)
	@which docker > /dev/null || (echo "❌ docker not found" && exit 1)

cluster:
	@echo "🚀 Starting Minikube..."
	@minikube status | grep "Running" || minikube start --cpus 4 --memory 6144 --driver=docker

build:
	@echo "🐳 Building Docker Images..."
	@./scripts/setup.sh

infra:
	@echo "🏗️  Applying Terraform (ArgoCD)..."
	@cd terraform && terraform init && terraform apply -auto-approve

deploy:
	@echo "📦 Deploying App-of-Apps..."
	@# 1. Update Repo URL placeholder if needed (optional sed command)
	@# 2. Deploy Infra (Redis/Postgres)
	@kubectl apply -f k8s/infra/project.yaml
	@echo "⏳ Waiting for Infra to sync..."
	@sleep 10
	@# 3. Deploy Observability (LGTM)
	@kubectl apply -f k8s/observability/project.yaml
	@echo "⏳ Waiting for Observability to sync..."
	@sleep 10
	@# 4. Deploy Apps
	@kubectl apply -f k8s/apps/ecommerce-demo.yaml

dashboards:
	@echo "📊 Uploading Dashboards..."
	@# Ensure namespace exists just in case ArgoCD is slow
	@kubectl get namespace monitoring > /dev/null 2>&1 || kubectl create namespace monitoring
	@kubectl apply -f k8s/observability/dashboards/red-dashboard.yaml

forward-argocd:
	@echo "🔌 Port Forwarding ArgoCD..."
	@echo "   👉 Open http://localhost:8080 (User: admin)"
	@echo "   Get password: kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
	@kubectl port-forward svc/argocd-server -n argocd 8080:443

forward-grafana:
	@echo "🔌 Port Forwarding Grafana..."
	@echo "   👉 Open http://localhost:3000 (User: admin / Pass: admin)"
	@kubectl port-forward svc/grafana -n monitoring 3000:80

clean:
	@echo "💥 Destroying Environment..."
	@minikube delete
	@rm -rf terraform/.terraform terraform/.terraform.lock.hcl terraform/terraform.tfstate*