type PostHogInstance = typeof import("posthog-js").default;

let _posthog: PostHogInstance | null = null;

export const getPostHog = (): PostHogInstance | null => _posthog;

export const setPostHog = (instance: PostHogInstance): void => {
  _posthog = instance;
};
