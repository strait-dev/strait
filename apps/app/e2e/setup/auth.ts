import { chromium } from "@playwright/test";

export async function signInAndSaveState(
  baseURL: string,
  email: string,
  password: string,
  storageStatePath = "playwright/.auth/user.json"
) {
  const signinRes = await fetch(`${baseURL}/api/auth/sign-in/email`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Origin: baseURL },
    body: JSON.stringify({ email, password }),
    redirect: "manual",
  });

  const setCookies = signinRes.headers.getSetCookie();
  const sessionCookie = setCookies.find((c) =>
    c.startsWith("better-auth.session_token=")
  );

  if (!sessionCookie) {
    const body = await signinRes.text();
    throw new Error(
      `Sign-in failed (${signinRes.status}): no session cookie. Body: ${body.slice(0, 200)}`
    );
  }

  const tokenMatch = sessionCookie.match(/better-auth\.session_token=([^;]+)/);
  if (!tokenMatch) {
    throw new Error("Could not parse session token from cookie");
  }

  // Set active organization
  const cookieHeader = `better-auth.session_token=${tokenMatch[1]}`;
  const sessionRes = await fetch(`${baseURL}/api/auth/session`, {
    headers: { Cookie: cookieHeader, Origin: baseURL },
  });
  if (sessionRes.ok) {
    const session = (await sessionRes.json()) as {
      user?: { defaultOrganizationId?: string };
    };
    const orgId = session?.user?.defaultOrganizationId;
    if (orgId) {
      await fetch(`${baseURL}/api/auth/organization/set-active`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Cookie: cookieHeader,
          Origin: baseURL,
        },
        body: JSON.stringify({ organizationId: orgId }),
      });
    }
  }

  // Verify session in browser and save storageState
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const cookieDomain = new URL(baseURL).hostname;
  await context.addCookies([
    {
      name: "better-auth.session_token",
      value: tokenMatch[1],
      domain: cookieDomain,
      path: "/",
      httpOnly: true,
      secure: false,
      sameSite: "Lax",
    },
  ]);

  const page = await context.newPage();
  try {
    await page.goto(`${baseURL}/app/dashboard`, {
      timeout: 90_000,
      waitUntil: "domcontentloaded",
    });
    await page.waitForURL("**/app/**", { timeout: 15_000 });
    await context.storageState({ path: storageStatePath });
  } finally {
    await browser.close();
  }
}
