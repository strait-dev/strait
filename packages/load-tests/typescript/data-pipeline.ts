/**
 * Data pipeline: fetch payload, generate 10K records, transform, aggregate, report output.
 *
 * Real data processing workload: generates synthetic data, groups by category,
 * computes aggregates (count, sum, avg, min, max), and reports results via SDK.
 */

const STRAIT_API = process.env.STRAIT_SDK_URL!;
const RUN_ID = process.env.STRAIT_RUN_ID!;
const TOKEN = process.env.STRAIT_RUN_TOKEN!;
const BASE = `${STRAIT_API}/sdk/v1/runs/${RUN_ID}`;
const HEADERS = {
  Authorization: `Bearer ${TOKEN}`,
  "Content-Type": "application/json",
};

interface DataRecord {
  id: number;
  value: number;
  category: string;
}

interface CategoryAggregate {
  category: string;
  count: number;
  sum: number;
  avg: number;
  min: number;
  max: number;
}

async function sdkPost(path: string, body: unknown): Promise<void> {
  await fetch(`${BASE}/${path}`, {
    method: "POST",
    headers: HEADERS,
    body: JSON.stringify(body),
  });
}

async function main(): Promise<void> {
  // Fetch payload
  // Fetch payload from SDK (simulates real job receiving work)
  await fetch(`${BASE}/payload`, { headers: HEADERS });

  // Report progress: starting
  await sdkPost("progress", { progress: 0.1, message: "Generating data" });

  // Generate 10K records
  const recordCount = Number(process.env.RECORD_COUNT) || 10000;
  const categories = ["A", "B", "C", "D", "E"];
  const data: DataRecord[] = Array.from({ length: recordCount }, (_, i) => ({
    id: i,
    value: Math.random() * 1000,
    category: categories[i % categories.length],
  }));

  await sdkPost("progress", { progress: 0.4, message: "Transforming data" });

  // Group by category and compute aggregates
  const grouped = new Map<string, DataRecord[]>();
  for (const record of data) {
    const existing = grouped.get(record.category);
    if (existing) {
      existing.push(record);
    } else {
      grouped.set(record.category, [record]);
    }
  }

  const result: CategoryAggregate[] = [];
  for (const [category, items] of grouped.entries()) {
    const values = items.map((item) => item.value);
    const sum = values.reduce((s, v) => s + v, 0);
    result.push({
      category,
      count: items.length,
      sum,
      avg: sum / items.length,
      min: Math.min(...values),
      max: Math.max(...values),
    });
  }

  await sdkPost("progress", { progress: 0.8, message: "Reporting results" });

  // Report usage (simulated)
  await sdkPost("usage", {
    provider: "openai",
    model: "gpt-4o-mini",
    prompt_tokens: 100,
    completion_tokens: 50,
  });

  // Report output
  await sdkPost("output", {
    output: {
      record_count: recordCount,
      categories: result.length,
      aggregates: result,
    },
  });

  // Complete
  await sdkPost("complete", {});

  await sdkPost("progress", { progress: 1.0, message: "Done" });
}

main().catch(async (err) => {
  try {
    await sdkPost("fail", { error: String(err) });
  } catch {
    // best effort
  }
  console.error(`FATAL: ${err}`);
  process.exit(1);
});
