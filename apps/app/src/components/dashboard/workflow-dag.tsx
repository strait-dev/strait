import { cn } from "@strait/ui/utils/index";
import type { StepRunStatus, WorkflowStepType } from "@/hooks/api/types";

type WorkflowStep = {
  id: string;
  name: string;
  type: WorkflowStepType;
  status: StepRunStatus;
  duration?: string;
  dependencies: string[];
};

type WorkflowDAGProps = {
  steps: WorkflowStep[];
  className?: string;
};

const STATUS_BORDER_COLORS: Record<StepRunStatus, string> = {
  completed: "var(--color-chart-1)",
  running: "var(--color-chart-3)",
  pending: "var(--color-chart-2)",
  waiting: "var(--color-chart-5)",
  failed: "var(--color-chart-4)",
  skipped: "var(--color-muted-foreground)",
  canceled: "var(--color-muted-foreground)",
};

const STATUS_BG_COLORS: Record<StepRunStatus, string> = {
  completed: "var(--color-chart-1)",
  running: "var(--color-chart-3)",
  pending: "var(--color-chart-2)",
  waiting: "var(--color-chart-5)",
  failed: "var(--color-chart-4)",
  skipped: "var(--color-muted-foreground)",
  canceled: "var(--color-muted-foreground)",
};

const TYPE_LABELS: Record<WorkflowStepType, string> = {
  job: "Job",
  approval: "Approval",
  sub_workflow: "Sub-Workflow",
  wait_for_event: "Wait",
  sleep: "Sleep",
};

const NODE_WIDTH = 200;
const NODE_HEIGHT = 80;
const H_GAP = 100;
const V_GAP = 40;
const NODE_PADDING_X = 60;
const NODE_PADDING_Y = 40;

/**
 * Topological sort into layers. Handles cycles by breaking them:
 * any node not placed after the main pass gets assigned to the last layer.
 */
/** Compute in-degree for each step, ignoring deps not present in byId. */
function computeInDegree(
  steps: WorkflowStep[],
  byId: Map<string, WorkflowStep>
): Map<string, number> {
  const inDegree = new Map<string, number>();
  for (const step of steps) {
    if (!inDegree.has(step.id)) {
      inDegree.set(step.id, 0);
    }
    for (const dep of step.dependencies) {
      if (byId.has(dep)) {
        inDegree.set(step.id, (inDegree.get(step.id) ?? 0) + 1);
      }
    }
  }
  return inDegree;
}

/**
 * Topological sort into layers. Handles cycles by breaking them:
 * any node not placed after the main pass gets assigned to the last layer.
 */
function topoLayers(steps: WorkflowStep[]): WorkflowStep[][] {
  const byId = new Map(steps.map((s) => [s.id, s]));
  const layers: WorkflowStep[][] = [];
  const placed = new Set<string>();

  const inDegree = computeInDegree(steps, byId);
  let frontier = steps.filter((s) => (inDegree.get(s.id) ?? 0) === 0);

  // Guard against max iterations to prevent infinite loops from cyclic deps
  let maxIter = steps.length + 1;
  while (frontier.length > 0 && maxIter-- > 0) {
    layers.push(frontier);
    for (const node of frontier) {
      placed.add(node.id);
    }

    const next: WorkflowStep[] = [];
    for (const step of steps) {
      if (placed.has(step.id)) {
        continue;
      }
      const remaining = step.dependencies.filter(
        (d) => byId.has(d) && !placed.has(d)
      );
      if (remaining.length === 0) {
        next.push(step);
      }
    }
    frontier = next;
  }

  // Cycle fallback: place any remaining nodes in a final layer
  const unplaced = steps.filter((s) => !placed.has(s.id));
  if (unplaced.length > 0) {
    layers.push(unplaced);
  }

  return layers;
}

function nodePosition(
  layerIdx: number,
  nodeIdx: number,
  nodesInLayer: number,
  totalHeight: number
) {
  const x = NODE_PADDING_X + layerIdx * (NODE_WIDTH + H_GAP);
  const layerHeight = nodesInLayer * NODE_HEIGHT + (nodesInLayer - 1) * V_GAP;
  const startY = (totalHeight - layerHeight) / 2;
  const y = startY + nodeIdx * (NODE_HEIGHT + V_GAP);
  return { x, y };
}

export function WorkflowDAG({ steps, className }: WorkflowDAGProps) {
  if (steps.length === 0) {
    return (
      <div
        className={cn(
          "flex items-center justify-center p-8 text-muted-foreground text-sm",
          className
        )}
      >
        No steps to display
      </div>
    );
  }

  const layers = topoLayers(steps);
  const maxPerLayer = Math.max(...layers.map((l) => l.length));
  const totalWidth =
    NODE_PADDING_X * 2 +
    layers.length * NODE_WIDTH +
    (layers.length - 1) * H_GAP;
  const totalHeight =
    NODE_PADDING_Y * 2 + maxPerLayer * NODE_HEIGHT + (maxPerLayer - 1) * V_GAP;

  // Build position lookup
  const positions = new Map<string, { x: number; y: number }>();
  for (let li = 0; li < layers.length; li++) {
    const layer = layers[li];
    for (let ni = 0; ni < layer.length; ni++) {
      positions.set(
        layer[ni].id,
        nodePosition(li, ni, layer.length, totalHeight)
      );
    }
  }

  // Build edges
  const edges: {
    from: { x: number; y: number };
    to: { x: number; y: number };
  }[] = [];
  for (const step of steps) {
    const target = positions.get(step.id);
    if (!target) {
      continue;
    }
    for (const depId of step.dependencies) {
      const source = positions.get(depId);
      if (!source) {
        continue;
      }
      edges.push({
        from: { x: source.x + NODE_WIDTH, y: source.y + NODE_HEIGHT / 2 },
        to: { x: target.x, y: target.y + NODE_HEIGHT / 2 },
      });
    }
  }

  return (
    <div
      className={cn("overflow-auto rounded-md border bg-muted/30", className)}
    >
      <svg
        height={totalHeight}
        viewBox={`0 0 ${totalWidth} ${totalHeight}`}
        width={totalWidth}
      >
        <defs>
          <marker
            id="dag-arrow"
            markerHeight="6"
            markerWidth="8"
            orient="auto-start-reverse"
            refX="10"
            refY="3.5"
            viewBox="0 0 10 7"
          >
            <polygon
              className="fill-muted-foreground"
              points="0 0, 10 3.5, 0 7"
            />
          </marker>
        </defs>

        {/* Edges */}
        {edges.map((edge, _i) => {
          const midX = (edge.from.x + edge.to.x) / 2;
          const d = `M ${edge.from.x} ${edge.from.y} C ${midX} ${edge.from.y}, ${midX} ${edge.to.y}, ${edge.to.x} ${edge.to.y}`;
          return (
            <path
              className="stroke-muted-foreground/50"
              d={d}
              fill="none"
              key={`${edge.from.x}-${edge.from.y}-${edge.to.x}-${edge.to.y}`}
              markerEnd="url(#dag-arrow)"
              strokeWidth={1.5}
            />
          );
        })}

        {/* Nodes */}
        {steps.map((step) => {
          const pos = positions.get(step.id);
          if (!pos) {
            return null;
          }
          const borderColor =
            STATUS_BORDER_COLORS[step.status] ?? STATUS_BORDER_COLORS.pending;
          const bgColor =
            STATUS_BG_COLORS[step.status] ?? STATUS_BG_COLORS.pending;

          return (
            <g key={step.id}>
              {/* Node background */}
              <rect
                className="fill-background"
                height={NODE_HEIGHT}
                rx={8}
                ry={8}
                stroke={borderColor}
                strokeWidth={2}
                width={NODE_WIDTH}
                x={pos.x}
                y={pos.y}
              />
              {/* Type indicator dot */}
              <circle
                cx={pos.x + 16}
                cy={pos.y + NODE_HEIGHT / 2}
                fill={bgColor}
                opacity={0.8}
                r={5}
              />
              {/* Name */}
              <text
                className="fill-foreground font-medium text-xs"
                fontSize={12}
                x={pos.x + 28}
                y={pos.y + NODE_HEIGHT / 2 - 6}
              >
                {step.name.length > 20
                  ? `${step.name.slice(0, 18)}...`
                  : step.name}
              </text>
              {/* Type + Duration */}
              <text
                className="fill-muted-foreground"
                fontSize={10}
                x={pos.x + 28}
                y={pos.y + NODE_HEIGHT / 2 + 10}
              >
                {TYPE_LABELS[step.type]}
                {step.duration ? ` - ${step.duration}` : ""}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}
