# Alert Rules

**Alerts are defined as code in `grafana-alerts.json`** in this directory.
Import them via: `apps/infra/scripts/import-alerts.sh`

Reference table below for quick lookup. Source of truth is the JSON file.

## Critical Alerts

| Alert | PromQL | For | Action |
|-------|--------|-----|--------|
| NodeNotReady | `kube_node_status_condition{condition="Ready",status="true"} == 0` | 5m | SSH into node, check k3s service. See RUNBOOK.md #1. |
| K3sProcessDown | `up{job="node-exporter"} == 0` | 3m | Node unreachable. Check Hetzner console. See RUNBOOK.md #2. |
| StraitDown | `up{job="strait"} == 0` | 2m | API server unreachable. Check pod status, logs. |

## Warning Alerts

| Alert | PromQL | For | Action |
|-------|--------|-----|--------|
| NodeDiskPressure | `(node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) < 0.15` | 10m | Clean up images, logs. See RUNBOOK.md #4. |
| NodeMemoryPressure | `(node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) < 0.10` | 5m | Check for memory leaks, consider larger nodes. |
| JobFailureSpike | `increase(kube_job_status_failed{namespace="default"}[5m]) > 10` | 0m | Check job logs, possible code or config issue. |
| PodCrashLoop | `increase(kube_pod_container_status_restarts_total{namespace=~"default|strait"}[15m]) > 5` | 0m | Check pod logs, OOM, config errors. |
| PodOOMKill | `kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} > 0` | 0m | Upgrade preset or increase memory limits. See RUNBOOK.md #6. |
| EtcdBackupStale | Custom: no successful backup log in 12h | 0m | Check etcd-backup timer, S3 credentials. See RUNBOOK.md #7. |

## Notification Channels

Configure in Grafana Cloud Alerting > Contact Points:
- **Critical**: PagerDuty or Slack #incidents
- **Warning**: Slack #monitoring or email

## Setup Instructions

1. Go to https://strait.grafana.net/alerting/list
2. Click "New alert rule" for each row above
3. Set the PromQL query as the condition
4. Set the "For" duration
5. Assign to the appropriate notification channel
