import { describe, expect, it } from "vitest";
import { seo } from "../seo";

type MetaTag =
  | { title: string }
  | { name: string; content: string }
  | { property: string; content: string };

function title(tags: MetaTag[]): string | undefined {
  const tag = tags.find((t): t is { title: string } => "title" in t);
  return tag?.title;
}

function property(tags: MetaTag[], key: string): string | undefined {
  const tag = tags.find(
    (t): t is { property: string; content: string } =>
      "property" in t && t.property === key
  );
  return tag?.content;
}

function name(tags: MetaTag[], key: string): string | undefined {
  const tag = tags.find(
    (t): t is { name: string; content: string } => "name" in t && t.name === key
  );
  return tag?.content;
}

describe("seo", () => {
  it("falls back to the bare site name when no title is given", () => {
    const tags = seo();
    expect(title(tags)).toBe("Strait");
    expect(property(tags, "og:title")).toBe("Strait");
    expect(name(tags, "twitter:title")).toBe("Strait");
  });

  it("appends the site name to a page title", () => {
    const tags = seo({ title: "Jobs" });
    expect(title(tags)).toBe("Jobs · Strait");
    expect(property(tags, "og:title")).toBe("Jobs · Strait");
    expect(name(tags, "twitter:title")).toBe("Jobs · Strait");
  });

  it("emits an absolute Open Graph image URL", () => {
    const image = property(seo(), "og:image");
    expect(image?.startsWith("http")).toBe(true);
    expect(image?.endsWith("/og.png")).toBe(true);
  });

  it("mirrors the OG image on the twitter card", () => {
    const tags = seo();
    expect(name(tags, "twitter:image")).toBe(property(tags, "og:image"));
    expect(name(tags, "twitter:card")).toBe("summary_large_image");
  });

  it("declares the shared image dimensions and site name", () => {
    const tags = seo();
    expect(property(tags, "og:image:width")).toBe("4800");
    expect(property(tags, "og:image:height")).toBe("2500");
    expect(property(tags, "og:site_name")).toBe("Strait");
  });

  it("uses the default description and lets callers override it", () => {
    const defaultDescription = name(seo(), "description");
    expect(defaultDescription).toContain("job orchestration");

    const tags = seo({ description: "Custom page description." });
    expect(name(tags, "description")).toBe("Custom page description.");
    expect(property(tags, "og:description")).toBe("Custom page description.");
    expect(name(tags, "twitter:description")).toBe("Custom page description.");
  });

  it("resolves a root-relative override image to an absolute URL", () => {
    const image = property(seo({ image: "/custom-og.png" }), "og:image");
    expect(image?.startsWith("http")).toBe(true);
    expect(image?.endsWith("/custom-og.png")).toBe(true);
  });

  it("leaves an already-absolute override image unchanged", () => {
    const remote = "https://cdn.example.com/share.png";
    expect(property(seo({ image: remote }), "og:image")).toBe(remote);
  });
});
