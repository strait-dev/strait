import { Badge } from "@strait/ui/components/badge";
import { Card, CardContent } from "@strait/ui/components/card";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { FeatureBadge } from "@strait/ui/components/feature-lock";
import { StatusBadge } from "@strait/ui/components/status-badge";
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
import { useEffect } from "react";
import type { StepRunStatus, WorkflowStepType } from "@/hooks/api/types";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { WorkflowIcon } from "@/lib/icons";
import {
  canUseFeature,
  getFeatureMinimumPlanLabel,
  type PlanFeature,
} from "@/lib/plan-tiers";

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

function renderFeatureBadge(currentPlan: string, feature: PlanFeature) {
  if (canUseFeature(currentPlan, feature)) {
    return null;
  }

  return (
    <FeatureBadge
      className="ml-1.5"
      plan={getFeatureMinimumPlanLabel(feature)}
      size="xs"
    />
  );
}

const WorkflowStepNode = ({ data }: { data: StepNodeData }) => {
  const currentPlan = useCurrentPlan();
  const badgeVariant = TYPE_BADGE_VARIANTS[data.stepType] ?? "secondary";

  return (
    <>
      <Handle position={Position.Top} type="target" />
      <Card className="min-w-[180px]" size="sm" variant="outline">
        <CardContent>
          <div className="mb-1.5 flex items-center gap-2">
            <StatusBadge dotOnly size="xs" status={data.status} />
            <span className="max-w-[150px] truncate font-medium text-card-foreground text-sm">
              {data.label}
            </span>
          </div>
          <span className="flex items-center">
            <Badge size="xs" variant={badgeVariant}>
              {TYPE_LABELS[data.stepType] ?? data.stepType}
            </Badge>
            {data.stepType === "approval" &&
              renderFeatureBadge(currentPlan, "approval_gates")}
            {data.stepType === "sub_workflow" &&
              renderFeatureBadge(currentPlan, "sub_workflows")}
          </span>
        </CardContent>
      </Card>
      <Handle position={Position.Bottom} type="source" />
    </>
  );
};

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

const WorkflowDAGFlow = ({ steps }: WorkflowDAGFlowProps) => {
  const { nodes: initialNodes, edges: initialEdges } =
    buildNodesAndEdges(steps);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  useEffect(() => {
    setNodes(initialNodes);
    setEdges(initialEdges);
  }, [initialNodes, initialEdges, setNodes, setEdges]);

  if (steps.length === 0) {
    return (
      <Card className="h-[400px] w-full" variant="outline">
        <CardContent className="flex h-full items-center justify-center">
          <ChartEmptyState
            icon={WorkflowIcon}
            message="Add workflow steps to display the execution graph."
            title="No steps to display"
          />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="h-[400px] w-full overflow-hidden" variant="outline">
      <CardContent className="h-full p-0">
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
          <Background color="var(--color-muted-foreground)" gap={20} size={1} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </CardContent>
    </Card>
  );
};

export default WorkflowDAGFlow;
