import {
  Folder01Icon,
  FolderLibraryIcon,
  RefreshIcon,
  Search01Icon,
} from "@hugeicons/core-free-icons";
import { dashboardHref } from "@/lib/urls";
import FeatureShowcase from "./feature-showcase";
import {
  DocumentsVisual1,
  DocumentsVisual2,
  DocumentsVisual3,
  DocumentsVisual4,
} from "./visuals/documents-visual";

const DocumentManagementShowcase = () => (
  <FeatureShowcase
    cta={{
      href: dashboardHref("/login"),
      label: "Manage your documents",
    }}
    description="Create, manage, and sync documents in real time — keep your writing projects organized and accessible from anywhere."
    features={[
      {
        title: "Create and organize documents",
        description:
          "Start new projects instantly and organize them with folders, tags, and custom metadata.",
        icon: Folder01Icon,
      },
      {
        title: "Auto-save with version history",
        description:
          "Never lose your work with automatic saves and the ability to restore previous versions.",
        icon: RefreshIcon,
      },
      {
        title: "Real-time sync",
        description:
          "Your documents sync instantly across all your devices — changes appear everywhere in real time.",
        icon: FolderLibraryIcon,
      },
      {
        title: "Quick search and filters",
        description:
          "Find any document in seconds with powerful search and filtering by date, title, or content.",
        icon: Search01Icon,
      },
    ]}
    title="Real-time sync, auto-save, and search across every draft"
    visuals={[
      <DocumentsVisual1 key="dv1" />,
      <DocumentsVisual2 key="dv2" />,
      <DocumentsVisual3 key="dv3" />,
      <DocumentsVisual4 key="dv4" />,
    ]}
  />
);

export default DocumentManagementShowcase;
