export type TocHeading = {
  id: string;
  text: string;
  level: number;
};

type RichTextNode = {
  type?: string;
  attrs?: { level?: number; id?: string };
  content?: RichTextNode[];
  text?: string;
  marks?: Array<{ type: string }>;
};

const NON_WORD_CHARS = /[^\w\s-]/g;
const WHITESPACE = /\s+/g;
const MULTIPLE_DASHES = /-+/g;
const WORD_BOUNDARY = /\s+/;

const slugify = (text: string): string =>
  text
    .toLowerCase()
    .replace(NON_WORD_CHARS, "")
    .replace(WHITESPACE, "-")
    .replace(MULTIPLE_DASHES, "-")
    .trim();

const extractTextFromNode = (node: RichTextNode): string => {
  if (node.text) {
    return node.text;
  }
  if (node.content) {
    return node.content.map(extractTextFromNode).join("");
  }
  return "";
};

export const extractHeadingsFromRichText = (
  content: RichTextNode[]
): TocHeading[] => {
  const headings: TocHeading[] = [];
  const seenSlugs = new Map<string, number>();

  const processNode = (node: RichTextNode) => {
    // Handle BaseHub heading format: type: "heading" with attrs.level
    if (node.type === "heading" && node.attrs?.level) {
      const level = node.attrs.level;
      if (level >= 2 && level <= 3) {
        const text = extractTextFromNode(node);
        // Use BaseHub's id if available, otherwise generate slug
        let slug = node.attrs.id ?? slugify(text);

        const count = seenSlugs.get(slug) ?? 0;
        if (count > 0) {
          slug = `${slug}-${count}`;
        }
        seenSlugs.set(node.attrs.id ?? slugify(text), count + 1);

        headings.push({ id: slug, text, level });
      }
    }

    if (node.content) {
      for (const child of node.content) {
        processNode(child);
      }
    }
  };

  for (const node of content) {
    processNode(node);
  }

  return headings;
};

const WORDS_PER_MINUTE = 200;

const countWordsInNode = (node: RichTextNode): number => {
  if (node.text) {
    return node.text.split(WORD_BOUNDARY).filter(Boolean).length;
  }
  if (node.content) {
    return node.content.reduce(
      (sum, child) => sum + countWordsInNode(child),
      0
    );
  }
  return 0;
};

export const countWords = (content: RichTextNode[]): number =>
  content.reduce((sum, node) => sum + countWordsInNode(node), 0);

export const calculateReadingTime = (content: RichTextNode[]): number => {
  const totalWords = countWords(content);
  return Math.max(1, Math.ceil(totalWords / WORDS_PER_MINUTE));
};

export const formatReadingTime = (minutes: number): string =>
  `${minutes} min read`;
