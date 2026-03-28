// @ts-expect-error fumadocs-core/server types not resolved in this tsconfig
import type { PageTree } from "fumadocs-core/server";
import { createFileRoute, notFound } from "@tanstack/react-router";
import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { source } from "@/lib/source";
import { useMemo } from "react";
import { docs } from "../../../source.generated";
import {
	DocsBody,
	DocsDescription,
	DocsPage,
	DocsTitle,
} from "fumadocs-ui/page";
import defaultMdxComponents from "fumadocs-ui/mdx";
// @ts-expect-error fumadocs-mdx/runtime/vite resolved by Vite at build time
import { createClientLoader } from "fumadocs-mdx/runtime/vite";
import { Step, Steps } from "fumadocs-ui/components/steps";
import { Tab, Tabs } from "fumadocs-ui/components/tabs";
import { Accordion, Accordions } from "fumadocs-ui/components/accordion";
import { Feedback } from "@/app/components/feedback";

const GITHUB_REPO = "https://github.com/leonardomso/strait";

export const Route = createFileRoute("/docs/$")({
	component: Page,
	loader: async ({ params }) => {
		const slugs = params._splat?.split("/") ?? [];
		const page = source.getPage(slugs);
		if (!page) {
			throw notFound();
		}
		const data = { tree: source.pageTree as object, path: page.path, slugs: page.slugs };
		await clientLoader.preload(data.path);
		return data;
	},
});

const clientLoader = createClientLoader(docs.doc, {
	id: "docs",
	component({ toc, frontmatter, default: MDX }: { toc: any; frontmatter: any; default: any }) {
		return (
			<DocsPage
				toc={toc}
				editOnGithub={{
					owner: "leonardomso",
					repo: "strait",
					path: `apps/docs/content/docs/${frontmatter._slugs?.join("/") ?? ""}.mdx`,
					sha: "master",
				}}
			>
				<DocsTitle>{frontmatter.title}</DocsTitle>
				{frontmatter.description && (
					<DocsDescription>{frontmatter.description}</DocsDescription>
				)}
				<DocsBody>
					<MDX
						components={{
							...defaultMdxComponents,
							Steps,
							Step,
							Tabs,
							Tab,
							Accordion,
							Accordions,
						}}
					/>
					<Feedback />
				</DocsBody>
			</DocsPage>
		);
	},
});

function Page() {
	const data = Route.useLoaderData();
	const Content = clientLoader.getComponent(data.path);
	const tree = useMemo(
		() => transformPageTree(data.tree as PageTree.Folder),
		[data.tree],
	);

	return (
		<DocsLayout
			tree={tree}
			nav={{
				title: "Strait Docs",
				url: "/",
			}}
			links={[
				{
					text: "GitHub",
					url: GITHUB_REPO,
					external: true,
				},
			]}
			githubUrl={GITHUB_REPO}
			sidebar={{
				tabs: [
					{ title: "Getting Started", url: "/docs/getting-started" },
					{ title: "Concepts", url: "/docs/concepts" },
					{ title: "SDKs", url: "/docs/sdks" },
					{ title: "Integrations", url: "/docs/integrations" },
					{ title: "AI Agents", url: "/docs/ai" },
					{ title: "API Reference", url: "/docs/api-reference" },
					{ title: "CLI", url: "/docs/cli" },
					{ title: "Guides", url: "/docs/guides" },
					{ title: "Operations", url: "/docs/operations" },
					{ title: "Development", url: "/docs/development" },
				],
			}}
		>
			<Content />
		</DocsLayout>
	);
}

function transformPageTree(tree: PageTree.Folder): PageTree.Folder {
	function transform<T extends PageTree.Item | PageTree.Separator>(item: T): T {
		if (typeof item.icon !== "string") {
			return item;
		}
		return {
			...item,
			icon: (
				<span
					// biome-ignore lint/security/noDangerouslySetInnerHtml: icon SVG from fumadocs
					dangerouslySetInnerHTML={{ __html: item.icon }}
				/>
			),
		};
	}

	return {
		...tree,
		index: tree.index ? transform(tree.index) : undefined,
		children: tree.children.map((item: any) => {
			if (item.type === "folder") {
				return transformPageTree(item);
			}
			return transform(item);
		}),
	};
}
