import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import type { AgentTemplate } from "@/hooks/api/use-agent-templates";

type TemplateCardProps = {
  template: AgentTemplate;
};

const CATEGORY_COLORS: Record<string, string> = {
  content: "bg-blue-50 text-blue-700 border-blue-200",
  engineering: "bg-purple-50 text-purple-700 border-purple-200",
  operations: "bg-green-50 text-green-700 border-green-200",
};

/**
 * Renders a single agent template as a card with name, description,
 * category badge, model info, and a "Use Template" action.
 */
function TemplateCard({ template }: TemplateCardProps) {
  return (
    <Card className="flex flex-col">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="text-base">{template.name}</CardTitle>
          <Badge
            className={CATEGORY_COLORS[template.category] ?? ""}
            variant="outline"
          >
            {template.category}
          </Badge>
        </div>
        <CardDescription className="line-clamp-2">
          {template.description}
        </CardDescription>
      </CardHeader>
      <CardContent className="mt-auto pt-0">
        <div className="flex items-center justify-between">
          <span className="font-mono text-muted-foreground text-xs">
            {template.model}
          </span>
          <a href={`/app/agents?template=${template.slug}`}>
            <Button size="sm" variant="outline">
              Use Template
            </Button>
          </a>
        </div>
      </CardContent>
    </Card>
  );
}

export default TemplateCard;
