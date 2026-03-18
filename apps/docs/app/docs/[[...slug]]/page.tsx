import { source } from "@/lib/source";
import { DocsPage, DocsBody, DocsDescription } from "fumadocs-ui/page";
import { notFound } from "next/navigation";
import { getMDXComponents } from "@/mdx-components";
import { Feedback } from "@/app/components/feedback";

type Props = {
  params: Promise<{ slug?: string[] }>;
};

export default async function Page({ params }: Props) {
  const { slug } = await params;
  const page = source.getPage(slug);
  if (!page) {
    notFound();
  }

  const MDX = page.data.body;

  return (
    <DocsPage
      toc={page.data.toc}
      editOnGithub={{
        owner: "leonardomso",
        repo: "strait",
        path: `apps/docs/content/docs/${page.slugs.join("/")}.mdx`,
        sha: "master",
      }}
    >
      <DocsBody>
        {page.data.description ? (
          <DocsDescription>{page.data.description}</DocsDescription>
        ) : null}
        <MDX components={getMDXComponents()} />
        <Feedback />
      </DocsBody>
    </DocsPage>
  );
}

export function generateStaticParams() {
  return source.generateParams();
}

export async function generateMetadata({ params }: Props) {
  const { slug } = await params;
  const page = source.getPage(slug);
  if (!page) {
    notFound();
  }

  const ogParams = new URLSearchParams({
    title: page.data.title,
    ...(page.data.description ? { description: page.data.description } : {}),
  });

  return {
    title: page.data.title,
    description: page.data.description,
    openGraph: {
      title: page.data.title,
      description: page.data.description,
      images: [`/api/og?${ogParams.toString()}`],
    },
    twitter: {
      card: "summary_large_image",
      title: page.data.title,
      description: page.data.description,
      images: [`/api/og?${ogParams.toString()}`],
    },
  };
}
