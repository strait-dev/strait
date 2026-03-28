import {
	createRootRoute,
	HeadContent,
	Outlet,
	Scripts,
} from "@tanstack/react-router";
import { RootProvider } from "fumadocs-ui/provider/base";
import { TanstackProvider } from "fumadocs-core/framework/tanstack";
import { cn } from "@strait/ui/utils";

import "../global.css";

export const Route = createRootRoute({
	component: RootComponent,
	head: () => ({
		meta: [
			{ charSet: "utf-8" },
			{ name: "viewport", content: "width=device-width, initial-scale=1, viewport-fit=cover" },
			{ title: "Strait Docs" },
			{ name: "description", content: "Documentation for Strait, the background job orchestration platform." },
			{ property: "og:type", content: "website" },
			{ property: "og:site_name", content: "Strait Docs" },
			{ property: "og:title", content: "Strait Docs" },
			{ property: "og:description", content: "Documentation for Strait, the background job orchestration platform." },
			{ property: "og:url", content: "https://docs.strait.dev" },
			{ name: "twitter:card", content: "summary_large_image" },
			{ name: "twitter:title", content: "Strait Docs" },
			{ name: "twitter:description", content: "Documentation for Strait, the background job orchestration platform." },
		],
		links: [
			{ rel: "icon", href: "/favicon.ico" },
			{ rel: "icon", href: "/favicon-32x32.png", sizes: "32x32", type: "image/png" },
			{ rel: "icon", href: "/favicon-16x16.png", sizes: "16x16", type: "image/png" },
		],
	}),
});

function RootComponent() {
	return (
		<html
			className={cn("min-h-screen bg-background antialiased")}
			lang="en-US"
			suppressHydrationWarning
		>
			<head>
				<HeadContent />
			</head>
			<body className="selection:bg-primary selection:text-primary-foreground">
				<TanstackProvider>
					<RootProvider
						theme={{
							attribute: "class",
							defaultTheme: "dark",
							disableTransitionOnChange: true,
							enableSystem: false,
							enableColorScheme: false,
						}}
					>
						<Outlet />
					</RootProvider>
				</TanstackProvider>
				<Scripts />
			</body>
		</html>
	);
}
