#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ITERATIONS=${1:-100}
TOTAL_PASS=0
TOTAL_FAIL=0
FAILED_ITERS=""
START=$(date +%s)

echo "=== Starting $ITERATIONS iteration stress test ==="
echo ""

for i in $(seq 1 "$ITERATIONS"); do
  if bash "$SCRIPT_DIR/smoke_test.sh" "$i" > /tmp/strait-smoke-$i.log 2>&1; then
    TOTAL_PASS=$((TOTAL_PASS + 1))
    if [ $((i % 10)) -eq 0 ] || [ "$i" -eq 1 ]; then
      ELAPSED=$(($(date +%s) - START))
      echo "  [iter $i/$ITERATIONS] PASS (${ELAPSED}s elapsed)"
    fi
  else
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    FAILED_ITERS="$FAILED_ITERS $i"
    echo "  [iter $i/$ITERATIONS] FAIL"
    tail -5 /tmp/strait-smoke-$i.log
  fi
done

ELAPSED=$(($(date +%s) - START))
echo ""
echo "=== Stress Test Results ==="
echo "Iterations: $ITERATIONS"
echo "Passed:     $TOTAL_PASS"
echo "Failed:     $TOTAL_FAIL"
echo "Duration:   ${ELAPSED}s"
echo "Avg:        $(echo "scale=2; $ELAPSED / $ITERATIONS" | bc)s/iter"
if [ -n "$FAILED_ITERS" ]; then
  echo "Failed iterations:$FAILED_ITERS"
fi

# Check service memory usage
echo ""
echo "=== Service Health ==="
STRAIT_PID=$(pgrep -f "go-build.*strait" 2>/dev/null | head -1 || echo "")
if [ -n "$STRAIT_PID" ]; then
  ps -o pid,rss,vsz,pcpu,pmem -p "$STRAIT_PID" 2>/dev/null || true
fi
curl -s http://localhost:8080/health/ready | python3 -m json.tool 2>/dev/null || echo "Service health check failed"

if [ "$TOTAL_FAIL" -gt 0 ]; then exit 1; fi
