import { ImageResponse } from "next/og";
import type { NextRequest } from "next/server";

export const runtime = "edge";

export function GET(request: NextRequest) {
  const { searchParams } = request.nextUrl;
  const title = searchParams.get("title") ?? "Strait Docs";
  const description =
    searchParams.get("description") ??
    "Background job orchestration for engineering teams and AI agents.";

  return new ImageResponse(
    <div
      style={{
        height: "100%",
        width: "100%",
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        padding: "80px",
        background: "linear-gradient(135deg, #0a0a0a 0%, #1a1a2e 100%)",
        color: "white",
        fontFamily: "system-ui, sans-serif",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "12px",
          marginBottom: "32px",
        }}
      >
        <div
          style={{
            width: "40px",
            height: "40px",
            borderRadius: "8px",
            background: "#3b82f6",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontSize: "20px",
            fontWeight: 700,
          }}
        >
          S
        </div>
        <span
          style={{
            fontSize: "20px",
            fontWeight: 500,
            opacity: 0.7,
          }}
        >
          Strait Docs
        </span>
      </div>
      <div
        style={{
          fontSize: "56px",
          fontWeight: 700,
          lineHeight: 1.15,
          marginBottom: "24px",
          maxWidth: "900px",
        }}
      >
        {title}
      </div>
      <div
        style={{
          fontSize: "24px",
          opacity: 0.6,
          lineHeight: 1.4,
          maxWidth: "700px",
        }}
      >
        {description}
      </div>
    </div>,
    {
      width: 1200,
      height: 630,
    }
  );
}
