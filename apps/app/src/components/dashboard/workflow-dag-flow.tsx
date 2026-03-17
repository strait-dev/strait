import { Badge } from "@strait/ui/components/badge";
import { cn } from "@strait/ui/utils/index";
import {
  Background,
  Controls,
  type Edge,
  Handle,
  type Node,
  Position,
  ReactFlow,
  useEdgesState,
  useNodesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useMemo } from "react";
import type { StepRunStatus, WorkflowStepType } from "@/hooks/api/types";

type WorkflowDAGFlowProps = {
  steps: {
    id: string;
    name: string;
    type: string;
    status: string;
    dependencies: string[];
  }[];
};

type StepNodeData = {
  label: string;
  stepType: WorkflowStepType;
  status: StepRunStatus;
};

const TYPE_LABELS: Record<WorkflowStepType, string> = {
  job: "Job",
  approval: "Approval",
  sub_workflow: "Sub-Workflow",
  wait_for_event: "Wait",
  sleep: "Sleep",
};

const TYPE_BORDER_COLORS: Record<WorkflowStepType, string> = {
  job: "border-l-chart-1",
  approval: "border-l-chart-3",
  sub_workflow: "border-l-chart-2",
  wait_for_event: "border-l-chart-5",
  sleep: "border-l-chart-4",
};

const STATUS_DOT_COLORS: Record<StepRunStatus, string> = {
  completed: "bg-success",
  running: "bg-info animate-pulse",
  pending: "bg-muted-foreground",
  waiting: "bg-warning",
  failed: "bg-destructive",
  skipped: "bg-muted-foreground",
  canceled: "bg-muted-foreground",
};

const TYPE_BADGE_VARIANTS: Record<
  WorkflowStepType,
  "default" | "secondary" | "outline"
> = {
  job: "secondary",
  approval: "outline",
  sub_workflow: "secondary",
  wait_for_event: "outline",
  sleep: "outline",
};

/** Custom node renderer for workflow steps */
function WorkflowStepNode({ data }: { data: StepNodeData }) {
  const borderClass =
    TYPE_BORDER_COLORS[data.stepType] ?? "border-l-muted-foreground";
  const dotColor = STATUS_DOT_COLORS[data.status] ?? "bg-muted-foreground";
  const badgeVariant = TYPE_BADGE_VARIANTS[data.stepType] ?? "secondary";

  return (
    <>
      <Handle
        className="!bg-muted-foreground/50 !w-2 !h-2 !border-0"
        position={Position.Top}
        type="target"
      />
      <div
        className={cn(
          "min-w-[180px] rounded-lg border border-border border-l-[3px] bg-card px-4 py-3 shadow-sm",
          "dark:border-border dark:bg-card",
          borderClass
        )}
      >
        <div className="mb-1.5 flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", dotColor)} />
          <span className="max-w-[150px] truncate font-medium text-card-foreground text-sm">
            {data.label}
          </span>
        </div>
        <Badge className="text-[10px]" size="xs" variant={badgeVariant}>
          {TYPE_LABELS[data.stepType] ?? data.stepType}
        </Badge>
      </div>
      <Handle
        className="!bg-muted-foreground/50 !w-2 !h-2 !border-0"
        position={Position.Bottom}
        type="source"
      />
    </>
  );
}

const nodeTypes = { workflowStep: WorkflowStepNode };

function computeInDegree<T extends { id: string; dependencies: string[] }>(
  steps: T[],
  knownIds: Set<string>
): Map<string, number> {
  const inDegree = new Map<string, number>();
  for (const step of steps) {
    inDegree.set(step.id, 0);
  }
  for (const step of steps) {
    for (const dep of step.dependencies) {
      if (knownIds.has(dep)) {
        inDegree.set(step.id, (inDegree.get(step.id) ?? 0) + 1);
      }
    }
  }
  return inDegree;
}

function findNextFrontier<T extends { id: string; dependencies: string[] }>(
  steps: T[],
  placed: Set<string>,
  knownIds: Set<string>
): T[] {
  return steps.filter((step) => {
    if (placed.has(step.id)) {
      return false;
    }
    return step.dependencies.every((d) => !knownIds.has(d) || placed.has(d));
  });
}

/** Topological sort into layers */
function topoLayers<T extends { id: string; dependencies: string[] }>(
  steps: T[]
): T[][] {
  const knownIds = new Set(steps.map((s) => s.id));
  const layers: T[][] = [];
  const placed = new Set<string>();

  const inDegree = computeInDegree(steps, knownIds);
  let frontier = steps.filter((s) => (inDegree.get(s.id) ?? 0) === 0);
  let maxIter = steps.length + 1;

  while (frontier.length > 0 && maxIter-- > 0) {
    layers.push(frontier);
    for (const node of frontier) {
      placed.add(node.id);
    }
    frontier = findNextFrontier(steps, placed, knownIds);
  }

  const unplaced = steps.filter((s) => !placed.has(s.id));
  if (unplaced.length > 0) {
    layers.push(unplaced);
  }

  return layers;
}

const NODE_WIDTH = 200;
const NODE_HEIGHT = 80;
const H_GAP = 120;
const V_GAP = 60;

function buildNodesAndEdges(steps: WorkflowDAGFlowProps["steps"]): {
  nodes: Node[];
  edges: Edge[];
} {
  if (steps.length === 0) {
    return { nodes: [], edges: [] };
  }

  const layers = topoLayers(steps);
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  for (let li = 0; li < layers.length; li++) {
    const layer = layers[li];
    for (let ni = 0; ni < layer.length; ni++) {
      const step = layer[ni];
      const x = li * (NODE_WIDTH + H_GAP);
      const layerHeight =
        layer.length * NODE_HEIGHT + (layer.length - 1) * V_GAP;
      const startY = 0 - layerHeight / 2;
      const y = startY + ni * (NODE_HEIGHT + V_GAP);

      nodes.push({
        id: step.id,
        type: "workflowStep",
        position: { x, y },
        data: {
          label: step.name,
          stepType: step.type as WorkflowStepType,
          status: step.status as StepRunStatus,
        } satisfies StepNodeData,
      });

      for (const depId of step.dependencies) {
        if (steps.some((s) => s.id === depId)) {
          edges.push({
            id: `${depId}-${step.id}`,
            source: depId,
            target: step.id,
            type: "smoothstep",
            animated: true,
            style: {
              stroke: "var(--color-muted-foreground)",
              strokeWidth: 1.5,
              opacity: 0.5,
            },
          });
        }
      }
    }
  }

  return { nodes, edges };
}

export function WorkflowDAGFlow({ steps }: WorkflowDAGFlowProps) {
  const { initialNodes, initialEdges } = useMemo(() => {
    const { nodes, edges } = buildNodesAndEdges(steps);
    return { initialNodes: nodes, initialEdges: edges };
  }, [steps]);

  const [nodes, , onNodesChange] = useNodesState(initialNodes);
  const [edges, , onEdgesChange] = useEdgesState(initialEdges);

  if (steps.length === 0) {
    return (
      <div className="flex items-center justify-center p-8 text-muted-foreground text-sm">
        No steps to display
      </div>
    );
  }

  return (
    <div className="h-[400px] w-full rounded-lg">
      <ReactFlow
        edges={edges}
        fitView
        nodes={nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        nodeTypes={nodeTypes}
        onEdgesChange={onEdgesChange}
        onNodesChange={onNodesChange}
        proOptions={{ hideAttribution: true }}
      >
        <Background
          color="var(--color-muted-foreground)"
          gap={20}
          size={1}
          style={{ opacity: 0.3 }}
        />
        <Controls
          className="!bg-card !border-border !rounded-lg !shadow-sm [&>button]:!bg-card [&>button]:!border-border [&>button]:!text-foreground [&>button:hover]:!bg-muted"
          showInteractive={false}
        />
      </ReactFlow>
    </div>
  );
}
