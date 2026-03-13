import { passkeyClient } from "@better-auth/passkey/client";
import { polarClient } from "@polar-sh/better-auth";
import {
  magicLinkClient,
  oneTapClient,
  organizationClient,
} from "better-auth/client/plugins";
import { createAuthClient } from "better-auth/react";

const googleClientId = import.meta.env.VITE_GOOGLE_CLIENT_ID as
  | string
  | undefined;

/**
 * Better Auth client for browser-side authentication.
 * Handles: sign in/out, social auth, session management,
 * magic link, passkey, Google One Tap.
 */
export const authClient = createAuthClient({
  plugins: [
    organizationClient(),
    polarClient(),
    passkeyClient(),
    magicLinkClient(),
    ...(googleClientId ? [oneTapClient({ clientId: googleClientId })] : []),
  ],
});
