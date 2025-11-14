.PHONY: help cluster-create cluster-delete cluster-info test-fixtures-apply test-fixtures-cleanup test-fixtures-reset test-demo clean check-deps

CLUSTER_NAME ?= pv-safe-test
KUBECTL_CONTEXT = kind-$(CLUSTER_NAME)

help:
	@echo "pv-safe Development Makefile"
	@echo ""
	@echo "Cluster Management:"
	@echo "  make cluster-create          - Create kind cluster with configuration"
	@echo "  make cluster-delete          - Delete kind cluster"
	@echo "  make cluster-info            - Show cluster information"
	@echo ""
	@echo "Test Fixtures:"
	@echo "  make test-fixtures-apply     - Apply all test fixtures to cluster"
	@echo "  make test-fixtures-cleanup   - Remove all test fixtures from cluster"
	@echo "  make test-fixtures-reset     - Cleanup and reapply fixtures (full reset)"
	@echo ""
	@echo "Webhook Development:"
	@echo "  make webhook-build           - Build webhook Docker image and load into kind"
	@echo "  make webhook-deploy          - Deploy webhook to cluster"
	@echo "  make webhook-status          - Show webhook status and resources"
	@echo "  make webhook-logs            - Tail webhook logs"
	@echo "  make webhook-delete          - Delete webhook from cluster"
	@echo ""
	@echo "Testing:"
	@echo "  make test-demo               - Run interactive demo of test scenarios"
	@echo ""
	@echo "Complete Setup:"
	@echo "  make setup                   - Create cluster and apply fixtures (full setup)"
	@echo "  make teardown                - Cleanup fixtures and delete cluster (full cleanup)"
	@echo ""
	@echo "Utilities:"
	@echo "  make check-deps              - Check if required tools are installed"
	@echo "  make clean                   - Clean up local development artifacts"
	@echo ""

check-deps:
	@echo "Checking required dependencies..."
	@command -v kind >/dev/null 2>&1 || { echo "Error: kind is not installed. Visit https://kind.sigs.k8s.io/"; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { echo "Error: kubectl is not installed. Visit https://kubernetes.io/docs/tasks/tools/"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "Error: docker is not installed. Visit https://docs.docker.com/get-docker/"; exit 1; }
	@echo "All required dependencies are installed."

cluster-create: check-deps
	@chmod +x scripts/create-cluster.sh
	@scripts/create-cluster.sh

cluster-delete:
	@chmod +x scripts/delete-cluster.sh
	@scripts/delete-cluster.sh

cluster-info:
	@echo "Cluster: $(CLUSTER_NAME)"
	@echo "Context: $(KUBECTL_CONTEXT)"
	@echo ""
	@kubectl cluster-info --context $(KUBECTL_CONTEXT) 2>/dev/null || echo "Cluster not running"
	@echo ""
	@echo "Nodes:"
	@kubectl get nodes --context $(KUBECTL_CONTEXT) 2>/dev/null || echo "No nodes found"
	@echo ""
	@echo "Storage Classes:"
	@kubectl get storageclass --context $(KUBECTL_CONTEXT) 2>/dev/null || echo "No storage classes found"

test-fixtures-apply:
	@chmod +x scripts/apply-fixtures.sh
	@scripts/apply-fixtures.sh

test-fixtures-cleanup:
	@chmod +x scripts/cleanup-fixtures.sh
	@scripts/cleanup-fixtures.sh

test-fixtures-reset: test-fixtures-cleanup test-fixtures-apply
	@echo ""
	@echo "Test fixtures have been reset!"

test-demo:
	@echo "Running interactive test demo..."
	@echo ""
	@echo "This will demonstrate various test scenarios."
	@echo "Press Enter to continue through each step..."
	@echo ""
	@read -p "Press Enter to start..."
	@echo ""
	@echo "Step 1: Show all test namespaces"
	@kubectl get namespaces -l pv-safe-test=true
	@read -p "Press Enter to continue..."
	@echo ""
	@echo "Step 2: Show risky PVCs (Delete reclaim policy)"
	@kubectl get pvc -n test-risky -o wide
	@read -p "Press Enter to continue..."
	@echo ""
	@echo "Step 3: Show safe PVCs (Retain reclaim policy)"
	@kubectl get pvc -n test-safe -o wide
	@read -p "Press Enter to continue..."
	@echo ""
	@echo "Step 4: Show all PVs and their reclaim policies"
	@kubectl get pv -o custom-columns=NAME:.metadata.name,CAPACITY:.spec.capacity.storage,RECLAIM:.spec.persistentVolumeReclaimPolicy,STATUS:.status.phase,CLAIM:.spec.claimRef.name,STORAGECLASS:.spec.storageClassName
	@read -p "Press Enter to continue..."
	@echo ""
	@echo "Step 5: Try to delete a risky namespace (this should be blocked by pv-safe when installed)"
	@echo "Command: kubectl delete namespace test-risky --dry-run=server"
	@kubectl delete namespace test-risky --dry-run=server
	@echo ""
	@echo "Note: Currently succeeds because pv-safe webhook is not installed yet."
	@echo "Once pv-safe is installed, this deletion will be blocked."
	@read -p "Press Enter to continue..."
	@echo ""
	@echo "Step 6: Show production workload"
	@kubectl get pvc,pods -n test-production
	@echo ""
	@echo "Demo complete!"

setup: cluster-create test-fixtures-apply
	@echo ""
	@echo "Setup complete! Cluster is ready for testing."
	@echo ""
	@echo "Quick verification:"
	@kubectl get namespaces -l pv-safe-test=true
	@echo ""
	@echo "Next steps:"
	@echo "  - Run 'make test-demo' for an interactive demo"
	@echo "  - Run 'kubectl get pvc -A' to see all PVCs"
	@echo "  - Run 'kubectl get pv' to see all PVs"

teardown: test-fixtures-cleanup cluster-delete
	@echo ""
	@echo "Teardown complete!"

webhook-build:
	@chmod +x scripts/build-webhook.sh
	@scripts/build-webhook.sh

webhook-deploy:
	@chmod +x scripts/deploy-webhook.sh
	@scripts/deploy-webhook.sh

webhook-logs:
	@kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f

webhook-status:
	@echo "Webhook Status:"
	@echo "==============="
	@echo ""
	@echo "Pods:"
	@kubectl get pods -n pv-safe-system
	@echo ""
	@echo "Service:"
	@kubectl get svc -n pv-safe-system
	@echo ""
	@echo "Certificate:"
	@kubectl get certificate -n pv-safe-system
	@echo ""
	@echo "Webhook Configuration:"
	@kubectl get validatingwebhookconfiguration pv-safe-validating-webhook
	@echo ""
	@echo "To view logs:"
	@echo "  make webhook-logs"

webhook-delete:
	@echo "Deleting webhook..."
	@kubectl delete -f deploy/03-webhook-config.yaml --ignore-not-found=true
	@kubectl delete -f deploy/02-deployment.yaml --ignore-not-found=true
	@kubectl delete -f deploy/01-certificate.yaml --ignore-not-found=true
	@kubectl delete -f deploy/00-namespace.yaml --ignore-not-found=true
	@echo "Webhook deleted"

clean:
	@echo "Cleaning up local development artifacts..."
	@rm -rf bin/
	@rm -rf dist/
	@echo "Clean complete!"

kubeconfig:
	@kind get kubeconfig --name $(CLUSTER_NAME)

switch-context:
	@kubectl config use-context $(KUBECTL_CONTEXT)
	@echo "Switched to context: $(KUBECTL_CONTEXT)"
