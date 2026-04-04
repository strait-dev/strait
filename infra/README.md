# Strait Infrastructure — Hetzner + k3s

Production Kubernetes cluster for Strait managed job execution on Hetzner Cloud ARM servers.

## Architecture

```
Strait Server (Fly.io or self-hosted)
       │
       ├─��� COMPUTE_RUNTIME=k8s
       ├── K8S_KUBECONFIG=./infra/kubeconfig
       │
       ▼
k3s Cluster (Hetzner Cloud)
┌──────────────────────────────────┐
│  master: CAX21 (4 vCPU, 8GB)    ���  $7/mo
│  general-1: CAX21 (4 vCPU, 8GB) │  $7/mo — micro, small jobs
│  perf-1: CAX31 (8 vCPU, 16GB)   │  $14/mo — medium jobs
└──────────────────────────────────┘
   Total: ~$28/mo
```

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.5
- [Hetzner Cloud account](https://console.hetzner.cloud) with API token
- SSH key pair (`ssh-keygen -t ed25519`)
- `kubectl` installed locally

## Quick Start (Doppler)

```bash
# 1. Add Hetzner token to Doppler.
doppler secrets set HCLOUD_TOKEN --project strait --config dev

# 2. Deploy (one command).
cd infra
doppler run --project strait --config dev -- make infra-up

# 3. Connect Strait.
cd ../apps/strait
doppler run -- make dev  # K8S_KUBECONFIG injected via Doppler
```

## Quick Start (Manual)

```bash
cd infra

# 1. Configure.
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
# Edit terraform.tfvars — add your Hetzner API token and SSH key paths.

# 2. Deploy.
make infra-up

# 3. Connect Strait.
cd ../apps/strait
COMPUTE_RUNTIME=k8s K8S_KUBECONFIG=../../infra/kubeconfig make dev
```

## Commands

| Command | What It Does |
|---|---|
| `make infra-up` | Provision servers + install k3s + apply K8s manifests |
| `make infra-down` | Destroy all servers (requires confirmation) |
| `make infra-kubeconfig` | Fetch kubeconfig from master |
| `make infra-status` | Show server IPs + K8s node status |
| `make infra-ssh` | SSH into master node |
| `make infra-plan` | Preview Terraform changes |

## Node Pools

Jobs are routed to nodes via soft affinity based on the machine preset:

| Preset | Node Pool | Hetzner Type | Cost |
|---|---|---|---|
| micro, small-1x, small-2x | `general` | CAX21 (4 vCPU, 8GB) | $7/mo |
| medium-1x, medium-2x | `performance` | CAX31 (8 vCPU, 16GB) | $14/mo |
| large-1x, large-2x | `heavy` | CAX41 (16 vCPU, 32GB) | $27/mo |

Heavy nodes are disabled by default. Set `heavy_count = 1` in `terraform.tfvars` when needed.

## Scaling

Add more workers by changing counts in `terraform.tfvars`:

```hcl
general_count = 3   # 3 general workers
perf_count    = 2   # 2 performance workers
heavy_count   = 1   # 1 heavy worker
```

Then run `make infra-up` (Terraform applies the diff).

## Cost Estimates

| Configuration | Nodes | Monthly Cost |
|---|---|---|
| Minimum (default) | 1 master + 1 general + 1 perf | ~$28 |
| Balanced | 1 master + 2 general + 1 perf + 1 heavy | ~$62 |
| High volume | 1 master + 3 general + 2 perf + 1 heavy | ~$76 |

## Security

- Private network (10.0.0.0/16) for node-to-node traffic
- Firewall: SSH, K8s API, kubelet (private only), NodePort
- Job pods run as non-root with read-only filesystem
- No service account token mounted in job pods
- Network policy blocks access to cloud metadata services
