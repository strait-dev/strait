.PHONY: infra-up infra-down infra-kubeconfig infra-status infra-ssh infra-plan

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
