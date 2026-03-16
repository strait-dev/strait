import { CostBudgetExceededError } from "../errors";

export type CostBudgetOptions = {
  readonly maxCostMicrousd: number;
  readonly onWarning?: (current: number, max: number) => void;
  readonly warningThreshold?: number;
};

export type CostTracker = {
  readonly add: (costMicrousd: number) => void;
  readonly current: () => number;
  readonly remaining: () => number;
  readonly isExceeded: () => boolean;
};

export const createCostTracker = (options: CostBudgetOptions): CostTracker => {
  let currentCost = 0;
  const threshold = options.warningThreshold ?? 0.8;
  let warningFired = false;

  return {
    add: (costMicrousd) => {
      currentCost += costMicrousd;

      if (
        !warningFired &&
        options.onWarning &&
        currentCost >= options.maxCostMicrousd * threshold
      ) {
        warningFired = true;
        options.onWarning(currentCost, options.maxCostMicrousd);
      }

      if (currentCost >= options.maxCostMicrousd) {
        throw new CostBudgetExceededError({
          message: `Cost budget exceeded: ${currentCost} >= ${options.maxCostMicrousd} microusd`,
          currentCostMicrousd: currentCost,
          maxCostMicrousd: options.maxCostMicrousd,
        });
      }
    },
    current: () => currentCost,
    remaining: () => Math.max(0, options.maxCostMicrousd - currentCost),
    isExceeded: () => currentCost >= options.maxCostMicrousd,
  };
};

export const withCostBudget = <T>(
  fn: (tracker: CostTracker) => Promise<T>,
  options: CostBudgetOptions
): Promise<T> => {
  const tracker = createCostTracker(options);
  return fn(tracker);
};
