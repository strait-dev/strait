import { describe, expect, it } from "vitest";
import {
  applyLocalDefaults,
  parseDevServerOptions,
} from "../../../scripts/lib/local-bootstrap";

describe("parseDevServerOptions", () => {
  it("uses localhost defaults", () => {
    expect(parseDevServerOptions([])).toEqual({
      host: "localhost",
      port: "5173",
    });
  });

  it("derives host and port from vite args", () => {
    expect(
      parseDevServerOptions(["--host", "0.0.0.0", "--port", "5180"])
    ).toEqual({
      host: "localhost",
      port: "5180",
    });
  });
});

describe("applyLocalDefaults", () => {
  it("fills local-first defaults when env is missing", () => {
    const env = applyLocalDefaults({}, []);

    expect(env.AUTH_DATABASE_URL).toBe(
      "postgresql://strait:strait@localhost:5432/strait"
    );
    expect(env.BETTER_AUTH_URL).toBe("http://localhost:5173");
    expect(env.VITE_BASE_URL).toBe("http://localhost:5173");
    expect(env.DISABLE_EMAIL_VERIFICATION).toBe("true");
    expect(env.DISABLE_POLAR_BILLING).toBe("true");
    expect(env.LOCAL_DEV_USER_EMAIL).toBe("dev@local.strait");
  });

  it("respects explicit URLs while still applying the other defaults", () => {
    const env = applyLocalDefaults(
      {
        BETTER_AUTH_URL: "http://localhost:6001",
        VITE_BASE_URL: "http://localhost:6001",
      },
      ["--port", "5199"]
    );

    expect(env.BETTER_AUTH_URL).toBe("http://localhost:6001");
    expect(env.VITE_BASE_URL).toBe("http://localhost:6001");
    expect(env.LOCAL_DEV_USER_PASSWORD).toBe("devpassword123");
  });
});
