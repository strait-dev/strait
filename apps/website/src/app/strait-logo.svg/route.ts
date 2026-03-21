import { readFile } from "node:fs/promises";
import { join } from "node:path";

export async function GET() {
  const svg = await readFile(
    join(process.cwd(), "public", "strait.svg"),
    "utf-8"
  );

  return new Response(svg, {
    headers: {
      "Content-Type": "image/svg+xml",
      "Cache-Control": "public, max-age=31536000, immutable",
    },
  });
}
