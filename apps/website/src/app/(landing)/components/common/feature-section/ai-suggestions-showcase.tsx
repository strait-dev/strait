import {
  Chatting01Icon,
  MessageEdit01Icon,
  PencilEdit02Icon,
  SparklesIcon,
} from "@hugeicons/core-free-icons";
import { dashboardHref } from "@/lib/urls.ts";
import FeatureShowcase from "./feature-showcase.tsx";
import {
  ChatVisual1,
  ChatVisual2,
  ChatVisual3,
  ChatVisual4,
} from "./visuals/chat-visual.tsx";

const AISuggestionsShowcase = () => (
  <FeatureShowcase
    cta={{
      href: dashboardHref("/login"),
      label: "Start refining with AI",
    }}
    description="Ask the AI to adjust tone, restructure paragraphs, expand on ideas, or tighten your prose — all through natural conversation."
    features={[
      {
        title: "Chat alongside any draft",
        description:
          "Open a conversation next to your draft and ask for changes — the AI understands your full document context.",
        icon: Chatting01Icon,
      },
      {
        title: "Adjust tone and style in real time",
        description:
          'Request changes like "make this more conversational" or "add a stronger hook" and see updates instantly.',
        icon: PencilEdit02Icon,
      },
      {
        title: "Restructure and expand",
        description:
          "Ask the AI to reorganize sections, add supporting evidence, or expand bullet points into full paragraphs.",
        icon: MessageEdit01Icon,
      },
      {
        title: "Streaming responses",
        description:
          "See the AI's suggestions appear word by word as it processes your request — no waiting for batch results.",
        icon: SparklesIcon,
      },
    ]}
    orientation="visual-left"
    title="Chat with AI that reads your full draft, not just your prompt"
    visuals={[
      <ChatVisual1 key="cv1" />,
      <ChatVisual2 key="cv2" />,
      <ChatVisual3 key="cv3" />,
      <ChatVisual4 key="cv4" />,
    ]}
  />
);

export default AISuggestionsShowcase;
