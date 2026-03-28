// Prefixed with - to exclude from TanStack Router route tree
// This file is intended to be used as an API endpoint
import { source } from "@/lib/source";
import { createFromSource } from "fumadocs-core/search/server";

const searchServer = createFromSource(source, {
	language: "english",
});

export const searchHandler = searchServer;
