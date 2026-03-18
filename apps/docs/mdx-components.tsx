import defaultMdxComponents from "fumadocs-ui/mdx";
import { Step, Steps } from "fumadocs-ui/components/steps";
import { Tab, Tabs } from "fumadocs-ui/components/tabs";
import { Accordion, Accordions } from "fumadocs-ui/components/accordion";
import { createAPIPage } from "fumadocs-openapi/ui";
import { openapi } from "@/lib/openapi";
import type { MDXComponents } from "mdx/types";

const APIPage = createAPIPage(openapi, {
  playground: { enabled: true },
});

export function getMDXComponents(components?: MDXComponents): MDXComponents {
  return {
    ...defaultMdxComponents,
    Steps,
    Step,
    Tabs,
    Tab,
    Accordion,
    Accordions,
    APIPage,
    ...components,
  };
}

export const useMDXComponents = getMDXComponents;
