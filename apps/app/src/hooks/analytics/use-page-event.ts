import { useEffect, useRef } from "react";
import { getPostHog } from "@/lib/analytics";

export const usePageEvent = (
  event: string,
  properties?: Record<string, unknown>
) => {
  const firedRef = useRef(false);
  useEffect(() => {
    if (firedRef.current) {
      return;
    }
    firedRef.current = true;
    getPostHog()?.capture(event, properties);
  }, [event, properties]);
};
