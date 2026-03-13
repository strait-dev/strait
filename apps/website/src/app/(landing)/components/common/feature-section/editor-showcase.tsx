import {
  DownloadSquare02Icon,
  Folder01Icon,
  Tag01Icon,
  TextBoldIcon,
} from "@hugeicons/core-free-icons";
import { dashboardHref } from "@/lib/urls.ts";
import FeatureShowcase from "./feature-showcase.tsx";
import {
  EditorVisual1,
  EditorVisual2,
  EditorVisual3,
  EditorVisual4,
} from "./visuals/editor-visual.tsx";

const EditorShowcase = () => (
  <FeatureShowcase
    cta={{
      href: dashboardHref("/login"),
      label: "Try the editor",
    }}
    description="Write and organize with a rich-text editor featuring 20+ extensions, workspaces, folders, tags, and multi-format export."
    features={[
      {
        title: "Rich text with 20+ extensions",
        description:
          "Formatting, headings, lists, code blocks, images, links, and more — everything you need in one editor.",
        icon: TextBoldIcon,
      },
      {
        title: "Workspaces and folders",
        description:
          "Organize your writing projects into color-coded workspaces with nested folders.",
        icon: Folder01Icon,
      },
      {
        title: "Custom tags with color coding",
        description:
          "Create tags with colors to categorize and filter your documents, notes, and folders.",
        icon: Tag01Icon,
      },
      {
        title: "Export to PDF, DOCX, Markdown, and text",
        description:
          "Download your finished work in the format you need — ready to publish or share.",
        icon: DownloadSquare02Icon,
      },
    ]}
    title="Write, format, and organize — without switching tools"
    visuals={[
      <EditorVisual1 key="ev1" />,
      <EditorVisual2 key="ev2" />,
      <EditorVisual3 key="ev3" />,
      <EditorVisual4 key="ev4" />,
    ]}
  />
);

export default EditorShowcase;
