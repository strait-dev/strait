"use client";

import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";

type PostHogInstance = typeof import("posthog-js").default;

const PostHogContext = createContext<PostHogInstance | null>(null);

type PostHogProviderProps = {
  children: ReactNode;
};

export const PostHogProvider = ({ children }: PostHogProviderProps) => {
  const [client, setClient] = useState<PostHogInstance | null>(null);
  const initRef = useRef(false);

  useEffect(() => {
    if (initRef.current) {
      return;
    }
    initRef.current = true;

    const key = import.meta.env.VITE_POSTHOG_KEY;
    if (!key) {
      return;
    }

    const host = import.meta.env.VITE_POSTHOG_HOST;
    const isDevelopment = import.meta.env.DEV;

    import("posthog-js")
      .then(({ default: posthog }) => {
        if (!posthog.__loaded) {
          posthog.init(key, {
            api_host: host || "https://us.i.posthog.com",
            person_profiles: "identified_only",
            capture_pageview: isDevelopment ? false : "history_change",
            capture_pageleave: !isDevelopment,
            autocapture: !isDevelopment,
          });
        }
        setClient(posthog);
      })
      .catch((error: unknown) => {
        console.error("Failed to load PostHog:", error);
      });
  }, []);

  return (
    <PostHogContext.Provider value={client}>{children}</PostHogContext.Provider>
  );
};

export const usePostHog = (): PostHogInstance | null => {
  return useContext(PostHogContext);
};
