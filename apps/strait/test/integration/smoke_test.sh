#!/usr/bin/env bash
set -euo pipefail

API_KEY=$(cat /tmp/strait-test-api-key)
BASE="http://localhost:8080"
PROJECT_ID="test-project"
PASS=0
FAIL=0
ITER=${1:-1}

api() {
  curl -s -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" "$@"
}
api_code() {
  curl -so /dev/null -w '%{http_code}' -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" "$@"
}

ok()   { PASS=$((PASS+1)); }
fail() { FAIL=$((FAIL+1)); echo "  FAIL: $1"; }
check() {
  local desc="$1" code="$2" expected="$3"
  if [ "$code" = "$expected" ]; then ok; else fail "$desc (got $code, want $expected)"; fi
}

echo "=== Strait Integration Smoke Test (iteration $ITER) ==="

# 1. Health
echo "[1] Health"
check "GET /health" "$(api_code "$BASE/health")" "200"
check "GET /health/ready" "$(api_code "$BASE/health/ready")" "200"

# 2. Jobs CRUD
echo "[2] Jobs CRUD"
JOB_SLUG="smoke-job-$(date +%s)-$ITER"
JOB=$(api "$BASE/v1/jobs" -d "{
  \"project_id\": \"$PROJECT_ID\",
  \"slug\": \"$JOB_SLUG\",
  \"name\": \"Smoke Test Job $ITER\",
  \"endpoint_url\": \"https://httpbin.org/post\",
  \"timeout_secs\": 30,
  \"max_attempts\": 1
}")
JOB_ID=$(echo "$JOB" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])" 2>/dev/null || echo "")
if [ -n "$JOB_ID" ]; then ok; else fail "Create job: $JOB"; JOB_ID="skip"; fi

if [ "$JOB_ID" != "skip" ]; then
  check "GET job" "$(api_code "$BASE/v1/jobs/$JOB_ID")" "200"
  check "PATCH job" "$(api_code -X PATCH "$BASE/v1/jobs/$JOB_ID" -d '{"name":"Updated Smoke"}')" "200"
  check "GET job health" "$(api_code "$BASE/v1/jobs/$JOB_ID/health")" "200"
fi
check "List jobs" "$(api_code "$BASE/v1/jobs")" "200"

# 3. Trigger & Run lifecycle
echo "[3] Run lifecycle"
RUN_ID="skip"
if [ "$JOB_ID" != "skip" ]; then
  TRIGGER=$(api "$BASE/v1/jobs/$JOB_ID/trigger" -d '{"payload":{"test":true}}')
  RUN_ID=$(echo "$TRIGGER" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])" 2>/dev/null || echo "")
  if [ -n "$RUN_ID" ]; then ok; else fail "Trigger job: $TRIGGER"; RUN_ID="skip"; fi

  if [ "$RUN_ID" != "skip" ]; then
    check "GET run" "$(api_code "$BASE/v1/runs/$RUN_ID")" "200"
    check "GET run usage removed" "$(api_code "$BASE/v1/runs/$RUN_ID/usage")" "404"
    check "GET run tool-calls removed" "$(api_code "$BASE/v1/runs/$RUN_ID/tool-calls")" "404"
    check "GET run outputs" "$(api_code "$BASE/v1/runs/$RUN_ID/outputs")" "200"
    check "GET run events" "$(api_code "$BASE/v1/runs/$RUN_ID/events")" "200"
    check "GET run checkpoints" "$(api_code "$BASE/v1/runs/$RUN_ID/checkpoints")" "200"
    check "GET run state" "$(api_code "$BASE/v1/runs/$RUN_ID/state")" "200"
  fi
fi
check "List runs" "$(api_code "$BASE/v1/runs")" "200"
check "List runs ?status=queued" "$(api_code "$BASE/v1/runs?status=queued")" "200"
check "List runs ?error_class=timeout" "$(api_code "$BASE/v1/runs?error_class=timeout")" "200"
check "List DLQ runs" "$(api_code "$BASE/v1/runs/dlq")" "200"

# 4. Resource Monitoring (STR-133)
echo "[4] Resource monitoring"
if [ "$RUN_ID" != "skip" ]; then
  check "GET run resources" "$(api_code "$BASE/v1/runs/$RUN_ID/resources")" "200"
fi

# 5. Workflows
echo "[5] Workflows"
WF_ID="skip"
WF_RUN_ID="skip"
if [ "$JOB_ID" != "skip" ]; then
  WF_SLUG="smoke-wf-$(date +%s)-$ITER"
  WF=$(api "$BASE/v1/workflows" -d "{
    \"project_id\": \"$PROJECT_ID\",
    \"slug\": \"$WF_SLUG\",
    \"name\": \"Smoke Workflow $ITER\",
    \"steps\": [
      {\"step_ref\":\"step1\",\"step_type\":\"job\",\"job_id\":\"$JOB_ID\",\"depends_on\":[]},
      {\"step_ref\":\"step2\",\"step_type\":\"job\",\"job_id\":\"$JOB_ID\",\"depends_on\":[\"step1\"]}
    ]
  }")
  WF_ID=$(echo "$WF" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])" 2>/dev/null || echo "")
  if [ -n "$WF_ID" ]; then ok; else fail "Create workflow: $WF"; WF_ID="skip"; fi

  if [ "$WF_ID" != "skip" ]; then
    check "GET workflow" "$(api_code "$BASE/v1/workflows/$WF_ID")" "200"
    check "GET workflow graph" "$(api_code "$BASE/v1/workflows/$WF_ID/graph")" "200"
    check "GET workflow versions" "$(api_code "$BASE/v1/workflows/$WF_ID/versions")" "200"

    WF_TRIG=$(api "$BASE/v1/workflows/$WF_ID/trigger" -d '{"payload":{"wf_test":true}}' 2>/dev/null || echo "")
    WF_RUN_ID=$(echo "$WF_TRIG" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])" 2>/dev/null || echo "")
    if [ -n "$WF_RUN_ID" ]; then
      ok
      check "GET workflow run" "$(api_code "$BASE/v1/workflow-runs/$WF_RUN_ID")" "200"
      check "GET workflow run steps" "$(api_code "$BASE/v1/workflow-runs/$WF_RUN_ID/steps")" "200"
      check "GET workflow run graph" "$(api_code "$BASE/v1/workflow-runs/$WF_RUN_ID/graph")" "200"
      check "GET workflow run timeline" "$(api_code "$BASE/v1/workflow-runs/$WF_RUN_ID/timeline")" "200"
    else
      fail "Trigger workflow: $WF_TRIG"
    fi
  fi
fi
check "List workflows" "$(api_code "$BASE/v1/workflows")" "200"
check "List workflow runs" "$(api_code "$BASE/v1/workflow-runs")" "200"

# 6. Audit Events & Export (STR-116)
echo "[6] Audit events & export"
check "List audit events" "$(api_code "$BASE/v1/audit-events")" "200"
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
HOUR_AGO=$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "$NOW")
check "Export audit JSON" "$(api_code "$BASE/v1/audit-events/export?from=$HOUR_AGO&to=$NOW&format=json")" "200"
check "Export audit CSV" "$(api_code "$BASE/v1/audit-events/export?from=$HOUR_AGO&to=$NOW&format=csv")" "200"
check "Export audit NDJSON" "$(api_code "$BASE/v1/audit-events/export?from=$HOUR_AGO&to=$NOW&format=ndjson")" "200"
check "Export bad format 400" "$(api_code "$BASE/v1/audit-events/export?from=$HOUR_AGO&to=$NOW&format=xml")" "400"
check "Export missing params 400" "$(api_code "$BASE/v1/audit-events/export")" "400"

# 7. Notification Channels (STR-77)
echo "[7] Notification channels"
NC=$(api "$BASE/v1/notification-channels" -d '{
  "channel_type": "webhook",
  "name": "smoke-webhook",
  "config": {"url":"https://httpbin.org/post","secret":"test-secret"}
}')
NC_ID=$(echo "$NC" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])" 2>/dev/null || echo "")
if [ -n "$NC_ID" ]; then ok; else fail "Create notification channel: $NC"; NC_ID="skip"; fi

if [ "$NC_ID" != "skip" ]; then
  check "GET notification channel" "$(api_code "$BASE/v1/notification-channels/$NC_ID")" "200"
  check "PATCH notification channel" "$(api_code -X PATCH "$BASE/v1/notification-channels/$NC_ID" -d '{"name":"updated-webhook"}')" "200"
fi
check "List notification channels" "$(api_code "$BASE/v1/notification-channels")" "200"
check "List notification deliveries" "$(api_code "$BASE/v1/notification-deliveries")" "200"

# 8. Analytics (STR-106)
echo "[8] Analytics"
check "Performance analytics" "$(api_code "$BASE/v1/analytics/performance?period_hours=1")" "200"
check "Cost analytics" "$(api_code "$BASE/v1/analytics/costs?from=$HOUR_AGO&to=$NOW")" "200"
check "Cost trends" "$(api_code "$BASE/v1/analytics/costs/trends?from=$HOUR_AGO&to=$NOW&granularity=hourly")" "200"
check "Cost top" "$(api_code "$BASE/v1/analytics/costs/top?from=$HOUR_AGO&to=$NOW&limit=10")" "200"
check "Cost insights" "$(api_code "$BASE/v1/analytics/cost-insights?from=$HOUR_AGO&to=$NOW")" "200"

# 9. Stats & other endpoints
echo "[9] Stats & misc"
check "GET stats" "$(api_code "$BASE/v1/stats")" "200"
PQ="project_id=$PROJECT_ID"
check "List webhook subs" "$(api_code "$BASE/v1/webhooks/subscriptions?$PQ")" "200"
check "List webhook deliveries" "$(api_code "$BASE/v1/webhooks/deliveries?$PQ")" "200"
check "List API keys" "$(api_code "$BASE/v1/api-keys?$PQ")" "200"
check "List environments" "$(api_code "$BASE/v1/environments?$PQ")" "200"
check "List secrets" "$(api_code "$BASE/v1/secrets")" "200"
check "List roles" "$(api_code "$BASE/v1/roles")" "200"
check "List members" "$(api_code "$BASE/v1/members")" "200"
check "List log drains" "$(api_code "$BASE/v1/log-drains?$PQ")" "200"
check "List event sources" "$(api_code "$BASE/v1/event-sources?$PQ")" "200"
check "List event triggers" "$(api_code "$BASE/v1/events")" "200"
check "Event trigger stats" "$(api_code "$BASE/v1/events/stats")" "200"
check "List deployments" "$(api_code "$BASE/v1/deployments?$PQ")" "200"
check "List batch ops" "$(api_code "$BASE/v1/batch-operations?$PQ")" "200"

# 10. Job groups
echo "[10] Job groups"
GRP=$(api "$BASE/v1/job-groups" -d "{\"project_id\":\"$PROJECT_ID\",\"name\":\"smoke-group-$ITER\",\"slug\":\"smoke-grp-$(date +%s)-$ITER\"}")
GRP_ID=$(echo "$GRP" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])" 2>/dev/null || echo "")
if [ -n "$GRP_ID" ]; then ok; else fail "Create job group: $GRP"; fi
check "List job groups" "$(api_code "$BASE/v1/job-groups?$PQ")" "200"

# 11. Regions
echo "[11] Regions"
check "List regions" "$(api_code "$BASE/v1/regions")" "200"

# 12. Cleanup
echo "[12] Cleanup"
if [ "$NC_ID" != "skip" ]; then
  check "DELETE notification channel" "$(api_code -X DELETE "$BASE/v1/notification-channels/$NC_ID")" "204"
fi

# Summary
TOTAL=$((PASS + FAIL))
echo ""
echo "=== Results: $PASS/$TOTAL passed, $FAIL failed (iteration $ITER) ==="
if [ "$FAIL" -gt 0 ]; then exit 1; fi
