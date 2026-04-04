# Strait Infrastructure Runbook

Incident response procedures for the Hetzner k3s cluster. For each issue: Symptoms, Diagnosis, Fix, Prevention.

---

## 1. Node Not Ready

**Symptoms:** `kubectl get nodes` shows a node as NotReady. Alert: `NodeNotReady`.

**Diagnosis:**
```bash
kubectl describe node <node-name>          # Check conditions + events
ssh root@<node-ip> "systemctl status k3s"  # Or k3s-agent for workers
ssh root@<node-ip> "journalctl -u k3s -n 50"
```

**Fix:**
```bash
ssh root@<node-ip> "systemctl restart k3s"  # Or k3s-agent
# If restart fails, replace the node:
# 1. Drain: ./scripts/drain-node.sh <node-name>
# 2. Rebuild: make infra-up (Terraform recreates)
```

**Prevention:** Automatic OS updates, k3s auto-upgrades, monitoring alerts.

---

## 2. Master Down

**Symptoms:** `kubectl` commands fail. K8s API unreachable. Alert: `K3sProcessDown`.

**Diagnosis:**
```bash
ssh root@<master-ip> "systemctl status k3s"
ssh root@<master-ip> "df -h"  # Check disk space
ssh root@<master-ip> "free -m" # Check memory
```

**Fix (single master):**
```bash
# Option A: Restart k3s.
ssh root@<master-ip> "systemctl restart k3s"

# Option B: Restore from backup (if data corrupted).
# 1. Download latest snapshot from S3.
aws s3 ls s3://strait-backups/etcd-snapshots/ --endpoint-url <S3_ENDPOINT>
aws s3 cp s3://strait-backups/etcd-snapshots/<latest>.db /tmp/snapshot.db

# 2. SSH into master and restore.
ssh root@<master-ip>
k3s server --cluster-reset --etcd-arg="--snapshot-file=/tmp/snapshot.db"
systemctl restart k3s

# Option C: Full rebuild.
make infra-down && make infra-up
```

**Fix (HA, 3 servers):** Other 2 servers keep running. Replace failed server via `make infra-up`.

**Prevention:** HA mode (3 servers), etcd backups to S3 every 6 hours.

---

## 3. Worker Down

**Symptoms:** Pods on that worker go Pending. `kubectl get nodes` shows worker NotReady.

**Diagnosis:**
```bash
kubectl get pods -o wide | grep <worker-name>
ssh root@<worker-ip> "systemctl status k3s-agent"
```

**Fix:** Jobs auto-reschedule to other nodes. To replace:
```bash
make infra-up  # Terraform recreates the server
```

**Prevention:** Multiple workers per pool. Autoscaler provisions replacements.

---

## 4. Disk Full

**Symptoms:** Pods fail to start. `kubectl describe pod` shows "disk pressure". Alert: `NodeDiskPressure`.

**Diagnosis:**
```bash
ssh root@<node-ip> "df -h"
ssh root@<node-ip> "du -sh /var/lib/rancher/k3s/*"
ssh root@<node-ip> "du -sh /var/log/*"
```

**Fix:**
```bash
# Clean up old container images.
ssh root@<node-ip> "k3s crictl rmi --prune"

# Clean up old logs.
ssh root@<node-ip> "journalctl --vacuum-time=3d"

# Clean up completed job pods.
kubectl delete pods --field-selector=status.phase==Succeeded -n default
```

**Prevention:** TTLSecondsAfterFinished on jobs (600s), log rotation, monitoring alerts at 85%.

---

## 5. Jobs Stuck Pending

**Symptoms:** Jobs queued but pods stay Pending. `kubectl describe pod` shows scheduling errors.

**Diagnosis:**
```bash
kubectl describe pod <pod-name>
# Look for:
#   - Insufficient cpu/memory → add nodes
#   - node selector doesn't match → check pool labels
#   - pod has unbound PVC → check storage
```

**Fix:**
```bash
# Add more nodes.
# Edit terraform.tfvars: general_count = 2 (or perf_count, heavy_count)
make infra-up

# Or check node labels.
kubectl get nodes --show-labels | grep strait.dev/pool
```

**Prevention:** Cluster autoscaler, RuntimeRouter fallback to Fly.

---

## 6. OOM Kills

**Symptoms:** Pod exits with code 137. `kubectl describe pod` shows `OOMKilled`. Alert: `PodOOMKill`.

**Diagnosis:**
```bash
kubectl describe pod <pod-name> | grep -A5 "Last State"
# Check which preset was used vs actual memory consumption.
```

**Fix:** The K8sRuntime auto-detects OOM and can trigger preset auto-upgrade for the next run (via `_preset_override` metadata). User can also manually set a larger preset.

**Prevention:** Accurate preset sizing, OOM auto-upgrade, memory monitoring.

---

## 7. Etcd Corruption

**Symptoms:** K8s API returns inconsistent data. k3s logs show etcd errors.

**Diagnosis:**
```bash
ssh root@<master-ip> "journalctl -u k3s | grep -i etcd | tail -20"
```

**Fix:**
```bash
# 1. Stop k3s.
ssh root@<master-ip> "systemctl stop k3s"

# 2. List available snapshots.
ssh root@<master-ip> "ls -la /var/lib/rancher/k3s/server/db/snapshots/"
# Or from S3:
aws s3 ls s3://strait-backups/etcd-snapshots/ --endpoint-url <S3_ENDPOINT>

# 3. Restore from latest good snapshot.
ssh root@<master-ip> "k3s server --cluster-reset --etcd-arg='--snapshot-file=/var/lib/rancher/k3s/server/db/snapshots/<snapshot>'"

# 4. Restart k3s.
ssh root@<master-ip> "systemctl start k3s"

# 5. Verify.
kubectl get nodes
kubectl get pods -A
```

**Prevention:** Automatic etcd snapshots every 6 hours, S3 backup sync, HA mode.

---

## 8. Security Incident

**Symptoms:** Unexpected pods, unusual network traffic, modified files, unauthorized access.

**Diagnosis:**
```bash
# Check for unknown pods.
kubectl get pods -A | grep -v "kube-system\|monitoring\|system-upgrade"

# Check k3s audit log.
ssh root@<master-ip> "tail -100 /var/log/k3s-audit.log"

# Check SSH login attempts.
ssh root@<node-ip> "journalctl -u sshd | grep -i fail | tail -20"
ssh root@<node-ip> "fail2ban-client status sshd"
```

**Fix:**
```bash
# 1. Isolate compromised node.
kubectl cordon <node-name>
kubectl drain <node-name> --ignore-daemonsets --force

# 2. Rotate ALL credentials.
# - Hetzner API token (regenerate in console)
# - SSH keys (generate new pair, update Terraform)
# - K3s node token (k3s token rotate on master)
# - Grafana Cloud API keys (regenerate)
# - Doppler secrets (rotate affected keys)

# 3. Rebuild the node.
# Delete the compromised server in Hetzner console.
make infra-up  # Terraform recreates with fresh OS.

# 4. Audit.
# Review k3s audit logs for unauthorized API calls.
# Review fail2ban logs for brute force patterns.
# Check if any job pods had unexpected behavior.
```

**Prevention:** SSH hardening, fail2ban, secrets encryption, network policy, minimal RBAC, regular credential rotation.

---

## 9. Hetzner Outage

**Symptoms:** All nodes unreachable. Hetzner status page shows incident.

**Diagnosis:**
```bash
# Check Hetzner status.
curl -s https://status.hetzner.com/api/v1/components.json | jq '.data[] | select(.status != "operational")'
```

**Fix:**
```bash
# Failover to Fly.io.
# Set COMPUTE_RUNTIME=fly (or COMPUTE_FALLBACK_PROVIDER=fly handles this automatically).
# If using RuntimeRouter, jobs automatically fall back to Fly when K8s is unreachable.
```

**Prevention:** RuntimeRouter with Fly fallback, multi-region Hetzner (if critical).

---

## 10. k3s Upgrade Failure

**Symptoms:** System Upgrade Controller stuck. Node in upgrade state. Alert: `NodeNotReady` during upgrade.

**Diagnosis:**
```bash
kubectl get plans -n system-upgrade
kubectl describe plan k3s-server -n system-upgrade
kubectl get pods -n system-upgrade
```

**Fix:**
```bash
# 1. Check upgrade pod logs.
kubectl logs -n system-upgrade -l upgrade.cattle.io/plan=k3s-server

# 2. If stuck, manually upgrade.
ssh root@<node-ip> "curl -sfL https://get.k3s.io | sh -"

# 3. If broken, rollback.
ssh root@<node-ip> "systemctl stop k3s"
ssh root@<node-ip> "cp /usr/local/bin/k3s.bak /usr/local/bin/k3s"  # If backup exists
ssh root@<node-ip> "systemctl start k3s"

# 4. If all else fails, rebuild.
make infra-down && make infra-up
```

**Prevention:** System Upgrade Controller upgrades one node at a time. HA mode means the cluster stays up during upgrades.
