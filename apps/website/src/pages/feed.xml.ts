import type { APIRoute } from "astro";
import { basehub } from "basehub";

const BASE_URL = import.meta.env.PUBLIC_WEBSITE_URL || "https://trystrait.ai";

function extractTextFromRichText(content: unknown[]): string {
  let text = "";

  for (const node of content) {
    if (typeof node !== "object" || node === null) {
      continue;
    }

    const n = node as Record<string, unknown>;

    if (n.type === "text" && typeof n.text === "string") {
      text += n.text;
    }

    if (Array.isArray(n.content)) {
      text += extractTextFromRichText(n.content);
    }

    if (n.type === "paragraph" || n.type === "heading") {
      text += "\n\n";
    }
  }

  return text.trim();
}

function escapeXml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

export const GET: APIRoute = async () => {
  let posts: any[] = [];

  try {
    const data = await basehub().query({
      website: {
        blog: {
          posts: {
            __args: {
              orderBy: "publishedAt__DESC",
              first: 50,
            },
            items: {
              _id: true,
              _title: true,
              _slug: true,
              description: true,
              publishedAt: true,
              categories: true,
              authors: {
                _title: true,
              },
              body: {
                json: {
                  content: true,
                },
              },
            },
          },
        },
      },
    });

    posts = (data as any).website.blog.posts.items.filter(
      (post: any) => post._slug !== "nos-somos-a-strait"
    );
  } catch {
    // If basehub is unavailable, return empty feed
  }

  const rssItems = posts
    .map((post: any) => {
      const pubDate = post.publishedAt
        ? new Date(post.publishedAt).toUTCString()
        : new Date().toUTCString();

      const categories = post.categories
        ?.map((cat: string) => `<category>${escapeXml(cat)}</category>`)
        .join("\n        ");

      const author = post.authors?.[0]?._title;

      const fullContent = post.body?.json?.content
        ? extractTextFromRichText(post.body.json.content as unknown[])
        : "";

      return `    <item>
      <title>${escapeXml(post._title)}</title>
      <link>${BASE_URL}/blog/${post._slug}</link>
      <guid isPermaLink="true">${BASE_URL}/blog/${post._slug}</guid>
      <description>${escapeXml(post.description ?? "")}</description>
      <pubDate>${pubDate}</pubDate>
      ${categories ?? ""}
      ${author ? `<author>${escapeXml(author)}</author>` : ""}
      ${fullContent ? `<content:encoded><![CDATA[${fullContent}]]></content:encoded>` : ""}
    </item>`;
    })
    .join("\n");

  const rss = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Strait Blog</title>
    <link>${BASE_URL}/blog</link>
    <description>Insights, tips, and best practices for writers using AI to improve their craft.</description>
    <language>en-us</language>
    <lastBuildDate>${new Date().toUTCString()}</lastBuildDate>
    <atom:link href="${BASE_URL}/feed.xml" rel="self" type="application/rss+xml"/>
    <image>
      <url>${BASE_URL}/android-chrome-512x512.png</url>
      <title>Strait Blog</title>
      <link>${BASE_URL}/blog</link>
    </image>
${rssItems}
  </channel>
</rss>`;

  return new Response(rss, {
    headers: {
      "Content-Type": "application/rss+xml; charset=utf-8",
      "Cache-Control": "public, max-age=3600, s-maxage=3600",
    },
  });
};
