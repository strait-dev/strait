import { useEffect } from "react";
import { authClient } from "@/lib/auth-client";

const googleClientId = import.meta.env.VITE_GOOGLE_CLIENT_ID as
  | string
  | undefined;

export const OneTapInitializer = () => {
  useEffect(() => {
    if (!googleClientId) {
      return;
    }
    authClient.oneTap().catch(() => {
      // Google One Tap may fail silently (blocked by browser, not loaded, etc.)
    });
  }, []);

  return null;
};
