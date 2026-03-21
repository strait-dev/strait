# Better Stack + Grafana Cloud Alerting Setup

This runbook documents the manual configuration for Better Stack (uptime,
alerting, on-call) and Grafana Cloud Alerting integration.

## 1. Better Stack Uptime Monitor

1. Log in to Better Stack (https://betterstack.com)
2. Go to **Monitors** > **Create monitor**
3. Configure:
   - **URL**: `https://strait.fly.dev/health`
   - **Check interval**: 60 seconds
   - **Request method**: GET
   - **Expected status code**: 200
4. Assign to the on-call escalation policy (see section 4)

## 2. Grafana Cloud Webhook Integration

1. In Better Stack, go to **Integrations** > **Add integration**
2. Select **Grafana** integration type
3. Copy the webhook URL (format: `https://uptime.betterstack.com/api/v1/incoming-webhook/XXXX`)
4. In Grafana Cloud, go to **Alerting** > **Contact points**
5. Click **Add contact point**:
   - **Name**: `Better Stack`
   - **Type**: Webhook
   - **URL**: paste the Better Stack webhook URL from step 3
   - **HTTP method**: POST
6. Test the contact point to verify delivery

## 3. Alert Routing

1. In Grafana Cloud, go to **Alerting** > **Notification policies**
2. Set the **default** notification policy to use the `Better Stack` contact point
3. Import Prometheus alert rules from `ops/monitoring/alerts-*.yaml`:
   - Go to **Alerting** > **Alert rules** > **New alert rule**
   - Select **Grafana-managed** rule type
   - Use the Prometheus datasource
   - Copy each `expr` from the YAML files into individual rules
   - Set `for` duration and severity labels to match
   - Alternatively, if using Grafana Mimir, load the YAML files directly as ruler rules

### Alert rule files

- `ops/monitoring/alerts-authz-rbac.yaml` -- RBAC/auth alerts (3 rules)
- `ops/monitoring/alerts-strait-core.yaml` -- Core operational alerts (14 rules)

## 4. On-Call Setup

1. In Better Stack, go to **On-call** > **Escalation policies**
2. Create a policy:
   - **Step 1**: Notify primary on-call via Slack + push notification
   - **Step 2** (after 5 min): Escalate to secondary on-call
   - **Step 3** (after 10 min): Notify entire team
3. Go to **On-call** > **Schedules** and create a rotation
4. Assign the escalation policy to the uptime monitor and Grafana integration

## 5. Notification Channels

Configure in Better Stack under **Settings** > **Notification preferences**:
- Slack: connect workspace and select alert channel
- Email: team member emails (auto-configured from accounts)
- Phone/SMS: add phone numbers for critical escalations

## Secrets Reference

| Secret | Location | Purpose |
|--------|----------|---------|
| `GRAFANA_LOKI_URL` | `fly secrets` on `strait-otel-collector` | Loki push endpoint |
| `GRAFANA_REMOTE_WRITE_URL` | `fly secrets` on `strait-otel-collector` | Prometheus remote write |
| `GRAFANA_REMOTE_WRITE_USERNAME` | `fly secrets` on `strait-otel-collector` | Grafana Cloud user ID |
| `GRAFANA_CLOUD_TOKEN` | `fly secrets` on `strait-otel-collector` | Grafana Cloud API key |
| Better Stack webhook URL | Grafana Cloud contact point | Alert delivery |
