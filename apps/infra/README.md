# Strait Infrastructure

Production Kubernetes cluster for Strait managed job execution.

Runs on [Hetzner Cloud](https://www.hetzner.com/cloud) ARM servers with [k3s](https://k3s.io) (lightweight Kubernetes). Fully automated provisioning via Terraform. Secrets managed by [Doppler](https://www.doppler.com).

---

## Architecture

```
Strait Server (managed infrastructure)
       |
       |--- COMPUTE_RUNTIME=k8s
       |--- K8S_KUBECONFIG=apps/infra/kubeconfig
       |
       v
k3s Cluster (Hetzner Cloud)
+--------------------------------------------------+
|  strait-master   CAX21  4 vCPU  8GB    $7/mo     |
|  strait-general  CAX21  4 vCPU  8GB    $7/mo     |
|  strait-perf     CAX31  8 vCPU  16GB   $14/mo    |
+--------------------------------------------------+
   Private network: 10.0.0.0/16
   Firewall: SSH, K8s API, kubelet (private), NodePort
   Total: ~$28/mo
```

### Node Pools

Jobs are routed to nodes via soft affinity based on the machine preset:

| Preset | Node Pool | Server Type | Cost | Use Case |
|--------|-----------|-------------|------|----------|
| micro, small-1x, small-2x | `general` | CAX21 (4 vCPU, 8GB ARM) | $7/mo | Lightweight jobs (~70% of volume) |
| medium-1x, medium-2x | `performance` | CAX31 (8 vCPU, 16GB ARM) | $14/mo | Data processing, ETL (~20%) |
| large-1x, large-2x | `heavy` | CAX41 (16 vCPU, 32GB ARM) | $27/mo | ML inference, builds (~10%) |

Heavy nodes are disabled by default (`heavy_count = 0`). Enable when needed.

---

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| [Terraform](https://developer.hashicorp.com/terraform/install) | >= 1.5 | `brew install terraform` |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | >= 1.28 | `brew install kubectl` |
| SSH key pair | ed25519 | `ssh-keygen -t ed25519` |
| [Hetzner Cloud account](https://console.hetzner.cloud) | -- | Sign up, create API token |
| [Doppler CLI](https://docs.doppler.com/docs/install-cli) (optional) | -- | `brew install doppler` |

---

## Quick Start

### Option 1: With Doppler (recommended)

```bash
# Add your Hetzner token to Doppler.
doppler secrets set HCLOUD_TOKEN --project strait --config dev

# Deploy everything (one command).
cd apps/infra
doppler run --project strait --config dev -- make infra-up

# Connect Strait to the cluster.
cd ../strait
doppler run -- make dev
```

### Option 2: Manual (without Doppler)

```bash
cd apps/infra

# Configure.
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
# Edit terraform.tfvars: add your HCLOUD_TOKEN and SSH key paths.

# Deploy.
make infra-up

# Connect Strait.
cd ../strait
COMPUTE_RUNTIME=k8s K8S_KUBECONFIG=../infra/kubeconfig make dev
```

---

## Commands

All commands run from `apps/infra/`.

### Infrastructure Lifecycle

| Command | Description |
|---------|-------------|
| `make infra-up` | Provision servers + install k3s + apply K8s manifests |
| `make infra-down` | Destroy all servers and the cluster (requires confirmation) |
| `make infra-plan` | Preview Terraform changes without applying |
| `make infra-kubeconfig` | Fetch kubeconfig from the master node |

### Operations

| Command | Description |
|---------|-------------|
| `make infra-status` | Show server IPs, node status, and running jobs |
| `make infra-ssh` | SSH into the master node |
| `make infra-test` | Run infrastructure validation tests (6 tests) |
| `make infra-upgrade` | Install k3s auto-upgrade controller |

### Kubectl (after `make infra-kubeconfig`)

```bash
export KUBECONFIG=apps/infra/kubeconfig

kubectl get nodes -o wide           # Show all nodes
kubectl get jobs -n default          # Show running jobs
kubectl get pods -n default          # Show job pods
kubectl logs <pod-name>              # View job logs
kubectl top nodes                    # Node resource usage
```

---

## Scaling

Change node counts in `terraform.tfvars` (or Doppler env vars) and run `make infra-up`:

```hcl
general_count = 3   # 3 general workers ($21/mo)
perf_count    = 2   # 2 performance workers ($28/mo)
heavy_count   = 1   # 1 heavy worker ($27/mo)
```

| Configuration | Nodes | Monthly Cost | Handles |
|---------------|-------|-------------|---------|
| Minimum (default) | 3 (1 master + 1 general + 1 perf) | ~$28 | Up to 10K jobs/day |
| Balanced | 5 (1 master + 2 general + 1 perf + 1 heavy) | ~$62 | Up to 50K jobs/day |
| High volume | 7 (1 master + 3 general + 2 perf + 1 heavy) | ~$76 | Up to 100K+ jobs/day |

### Doppler Env Vars for Scaling

| Variable | Default | Description |
|----------|---------|-------------|
| `HCLOUD_TOKEN` | (required) | Hetzner API token |
| `SSH_KEY_PATH` | `~/.ssh/id_ed25519.pub` | SSH public key path |
| `SSH_PRIVATE_KEY_PATH` | `~/.ssh/id_ed25519` | SSH private key path |
| `HETZNER_LOCATION` | `fsn1` | Datacenter: fsn1 (Falkenstein), nbg1 (Nuremberg), hel1 (Helsinki) |
| `HETZNER_GENERAL_COUNT` | `1` | General pool workers |
| `HETZNER_PERF_COUNT` | `1` | Performance pool workers |
| `HETZNER_HEAVY_COUNT` | `0` | Heavy pool workers |

---

## Security

### Server Hardening (applied automatically via cloud-init)

- **SSH**: Password auth disabled, max 3 auth tries, no X11/TCP forwarding
- **fail2ban**: Auto-blocks brute force SSH attempts
- **Auto-updates**: `unattended-upgrades` installs OS security patches daily
- **Firewall**: Only SSH (22), K8s API (6443), kubelet (private network only), NodePort

### Kubernetes Security

- **Secrets encryption**: `--secrets-encryption` encrypts etcd secrets at rest
- **Kernel protection**: `--protect-kernel-defaults` enforces sysctl security
- **API audit log**: All K8s API calls logged to `/var/log/k3s-audit.log` (30 day retention)
- **Job pod hardening**: RunAsNonRoot, ReadOnlyRootFilesystem, drop ALL capabilities, no SA token
- **Network policy**: Blocks access to cloud metadata (169.254.169.254) and private IPs

### CI/CD Security

- **Terraform CI**: Every PR gets `terraform validate`, `fmt -check`, and `tfsec` security scan
- **Drift detection**: Nightly `terraform plan` creates GitHub Issue if infrastructure was changed outside Terraform
- **No auto-apply**: `terraform apply` is always manual

---

## DNS Setup

Caddy auto-provisions TLS certificates via Let's Encrypt, but it needs a domain. After deploying the cluster:

1. Set `strait_domain` in your `terraform.tfvars` (e.g., `api.yourdomain.com`)
2. Run `terraform apply` to see the `dns_instructions` output
3. Create a DNS A record in your DNS provider:

```
Type: A
Name: api (or your subdomain)
Value: <master_ip from terraform output>
TTL: 300
```

4. Create the Caddy domain secret:

```bash
kubectl -n strait create secret generic caddy-env \
  --from-literal=STRAIT_DOMAIN=api.yourdomain.com
```

5. Restart Caddy to pick up the domain:

```bash
kubectl -n strait rollout restart deployment/caddy
```

Caddy will automatically provision and renew the Let's Encrypt certificate.

---

## Infrastructure Tests

Run validation tests against the live cluster:

```bash
make infra-test
```

Tests verify:
- All nodes exist and are Ready
- `strait.dev/pool` labels set correctly on workers
- `strait-job` and `warm-pool` priority classes exist
- `strait-job-runner` service account exists (no RBAC)
- Resource quota applied
- A real K8s job can be created and completes successfully

---

## k3s Auto-Upgrades

Install the System Upgrade Controller for automatic k3s rolling upgrades:

```bash
make infra-upgrade
```

This installs a controller that watches for new k3s stable releases and performs rolling upgrades: drain node, upgrade k3s binary, uncordon. Server nodes upgrade first, then agents.

Check upgrade status:

```bash
kubectl --kubeconfig kubeconfig get plans -n system-upgrade
```

---

## Troubleshooting

### Servers not coming up

```bash
cd terraform && terraform show      # Check Terraform state
make infra-ssh                      # SSH into master
systemctl status k3s                # Check k3s service
journalctl -u k3s -f                # Stream k3s logs
```

### Workers not joining

```bash
ssh root@<worker-ip> "journalctl -u k3s-agent -f"    # Worker agent logs
ssh root@<master-ip> "cat /var/lib/rancher/k3s/server/node-token"  # Verify token
```

### Jobs stuck in Pending

```bash
kubectl describe pod <pod-name>
# Check Events section: insufficient resources, image pull error, unschedulable
```

### Reset everything

```bash
make infra-down    # Destroy all servers
make infra-up      # Rebuild from scratch
```

---

## File Structure

```
apps/infra/
  terraform/
    main.tf                    Hetzner provider configuration
    variables.tf               All variable declarations
    network.tf                 Private network (10.0.0.0/16)
    firewall.tf                Firewall rules
    ssh.tf                     SSH key upload
    servers.tf                 Server definitions (master, general, perf, heavy)
    cloud-init-master.yaml     Master bootstrap: SSH hardening + k3s server
    cloud-init-worker.yaml     Worker bootstrap: SSH hardening + k3s agent
    outputs.tf                 Server IPs, kubeconfig command, SSH command
    versions.tf                Terraform and provider versions
    terraform.tfvars.example   Example configuration (copy to terraform.tfvars)
  k8s/
    system-upgrade-controller.yaml   k3s auto-upgrade controller
    upgrade-plans.yaml               Server + agent upgrade plans
  scripts/
    setup.sh                  One-command deploy (Terraform + k3s + manifests)
    get-kubeconfig.sh         Fetch kubeconfig from master
    destroy.sh                Tear down everything (with confirmation)
  tests/
    cluster_test.go           Infrastructure validation tests
    go.mod                    Test module
  Makefile                    All infra commands
  README.md                   This file
```
