.PHONY: infra-up infra-down infra-kubeconfig infra-status infra-ssh infra-plan infra-test infra-upgrade

# Full setup: provision servers + install k3s + apply K8s manifests.
infra-up:
	@bash scripts/setup.sh

# Destroy all servers and the cluster.
infra-down:
	@bash scripts/destroy.sh

# Fetch kubeconfig from the master node.
infra-kubeconfig:
	@bash scripts/get-kubeconfig.sh

# Show cluster status.
infra-status:
	@echo "=== Hetzner Servers ==="
	@cd terraform && terraform output 2>/dev/null || echo "Not provisioned. Run: make infra-up"
	@echo ""
	@echo "=== K8s Nodes ==="
	@kubectl --kubeconfig kubeconfig get nodes -o wide 2>/dev/null || echo "No kubeconfig. Run: make infra-kubeconfig"
	@echo ""
	@echo "=== K8s Jobs ==="
	@kubectl --kubeconfig kubeconfig get jobs -n default 2>/dev/null || true

# SSH into the master node.
infra-ssh:
	@cd terraform && eval $$(terraform output -raw ssh_command)

# Preview changes without applying.
infra-plan:
	@cd terraform && terraform plan

# Run infrastructure validation tests against the live cluster.
infra-test:
	@cd tests && KUBECONFIG=../kubeconfig go test -v -timeout 5m ./...

# Apply k3s auto-upgrade controller and plans.
infra-upgrade:
	@kubectl --kubeconfig kubeconfig apply -f k8s/system-upgrade-controller.yaml
	@kubectl --kubeconfig kubeconfig apply -f k8s/upgrade-plans.yaml
	@echo "k3s auto-upgrade controller installed. Check: kubectl --kubeconfig kubeconfig get plans -A"
