import { StraitSDKError } from "./errors";
import type { JsonValue } from "./types";

export interface EvalAssertion<TResult> {
  assert: (result: TResult) => boolean | Promise<boolean>;
  message?: string;
  name: string;
}

export interface EvalJudgeResult {
  metadata?: JsonValue;
  passed: boolean;
  reason?: string;
  score?: number;
}

export interface EvalJudge<TResult> {
  judge: (result: TResult) => EvalJudgeResult | Promise<EvalJudgeResult>;
  name: string;
}

export interface EvalCase<TInput, TResult> {
  assertions?: EvalAssertion<TResult>[];
  input: TInput;
  judge?: EvalJudge<TResult>;
  metadata?: JsonValue;
  name: string;
  tags?: string[];
}

export interface EvalSuite<TInput, TResult> {
  cases: EvalCase<TInput, TResult>[];
  description?: string;
  name: string;
}

export interface EvalAssertionResult {
  message?: string;
  name: string;
  passed: boolean;
}

export interface EvalCaseResult<TResult> {
  assertions: EvalAssertionResult[];
  caseName: string;
  durationMs: number;
  error?: string;
  judge?: EvalJudgeResult & {
    name: string;
  };
  passed: boolean;
  result?: TResult;
  tags: string[];
}

export interface EvalSuiteResult<TResult> {
  cases: EvalCaseResult<TResult>[];
  durationMs: number;
  failed: number;
  name: string;
  passed: number;
  total: number;
}

function requireName(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

function normalizeTags(tags: string[] | undefined): string[] {
  if (tags == null) {
    return [];
  }
  return tags.map((tag, index) => requireName(tag, `tags[${index}]`));
}

export function defineEvalSuite<TInput, TResult>(
  suite: EvalSuite<TInput, TResult>
): EvalSuite<TInput, TResult> {
  const normalizedCases = suite.cases.map((testCase, caseIndex) => ({
    ...testCase,
    assertions: testCase.assertions?.map((assertion, assertionIndex) => ({
      ...assertion,
      name: requireName(assertion.name, `cases[${caseIndex}].assertions[${assertionIndex}].name`),
    })),
    name: requireName(testCase.name, `cases[${caseIndex}].name`),
    tags: normalizeTags(testCase.tags),
  }));

  if (normalizedCases.length === 0) {
    throw new StraitSDKError("eval suite requires at least one case");
  }

  const names = new Set<string>();
  for (const testCase of normalizedCases) {
    if (names.has(testCase.name)) {
      throw new StraitSDKError(`duplicate eval case name: ${testCase.name}`);
    }
    names.add(testCase.name);
  }

  return Object.freeze({
    ...suite,
    name: requireName(suite.name, "name"),
    cases: normalizedCases,
  });
}

export async function runEvalSuite<TInput, TResult>(
  suite: EvalSuite<TInput, TResult>,
  execute: (input: TInput, testCase: EvalCase<TInput, TResult>) => Promise<TResult>
): Promise<EvalSuiteResult<TResult>> {
  const normalizedSuite = defineEvalSuite(suite);
  const startedAt = Date.now();

  const caseResults = await Promise.all(
    normalizedSuite.cases.map(async (testCase) => {
      const caseStartedAt = Date.now();

      try {
        const result = await execute(testCase.input, testCase);
        const assertions = await Promise.all(
          (testCase.assertions ?? []).map(async (assertion) => ({
            name: assertion.name,
            message: assertion.message,
            passed: await assertion.assert(result),
          }))
        );

        const judgeResult = testCase.judge
          ? await testCase.judge.judge(result)
          : undefined;

        const passed =
          assertions.every((assertion) => assertion.passed) &&
          (judgeResult?.passed ?? true);

        return {
          caseName: testCase.name,
          durationMs: Date.now() - caseStartedAt,
          tags: testCase.tags ?? [],
          passed,
          result,
          assertions,
          judge: judgeResult
            ? {
                name: testCase.judge?.name ?? "judge",
                ...judgeResult,
              }
            : undefined,
        } satisfies EvalCaseResult<TResult>;
      } catch (error) {
        return {
          caseName: testCase.name,
          durationMs: Date.now() - caseStartedAt,
          tags: testCase.tags ?? [],
          passed: false,
          assertions: [],
          error: error instanceof Error ? error.message : String(error),
        } satisfies EvalCaseResult<TResult>;
      }
    })
  );

  const passed = caseResults.filter((result) => result.passed).length;

  return {
    name: normalizedSuite.name,
    cases: caseResults,
    total: caseResults.length,
    passed,
    failed: caseResults.length - passed,
    durationMs: Date.now() - startedAt,
  };
}

function getPathValue(value: unknown, path: string): unknown {
  return path.split(".").reduce<unknown>((current, segment) => {
    if (current == null || typeof current !== "object") {
      return undefined;
    }
    return (current as Record<string, unknown>)[segment];
  }, value);
}

export function expectPathEquals<TResult>(
  name: string,
  path: string,
  expected: JsonValue
): EvalAssertion<TResult> {
  return {
    name,
    message: `expected ${path} to equal ${JSON.stringify(expected)}`,
    assert: (result) => {
      return JSON.stringify(getPathValue(result, path)) === JSON.stringify(expected);
    },
  };
}

export function expectTextContains<TResult>(
  name: string,
  select: (result: TResult) => string,
  excerpts: string[]
): EvalAssertion<TResult> {
  const normalizedExcerpts = excerpts.map((excerpt, index) =>
    requireName(excerpt, `excerpts[${index}]`).toLowerCase()
  );

  return {
    name,
    message: `expected selected text to contain ${normalizedExcerpts.join(", ")}`,
    assert: (result) => {
      const text = select(result).toLowerCase();
      return normalizedExcerpts.every((excerpt) => text.includes(excerpt));
    },
  };
}

export function expectArrayMinLength<TResult, TValue>(
  name: string,
  select: (result: TResult) => TValue[],
  minLength: number
): EvalAssertion<TResult> {
  if (!Number.isInteger(minLength) || minLength < 0) {
    throw new StraitSDKError("minLength must be a non-negative integer");
  }

  return {
    name,
    message: `expected selected array length to be at least ${minLength}`,
    assert: async (result) => select(result).length >= minLength,
  };
}
