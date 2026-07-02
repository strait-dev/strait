const MINIMUM_SUBMIT_FEEDBACK_MS = 350;

export async function waitForMinimumSubmitFeedback(startedAt: number) {
  const remaining = MINIMUM_SUBMIT_FEEDBACK_MS - (Date.now() - startedAt);
  if (remaining > 0) {
    await new Promise((resolve) => setTimeout(resolve, remaining));
  }
}
