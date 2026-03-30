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
import type {
  AgentTopologyEdge,
  AgentTopologyNode,
} from "@/hooks/api/use-agents";

type AgentTopologyFlowProps = {
  edges: AgentTopologyEdge[];
  nodes: AgentTopologyNode[];
};

type AgentNodeData = {
  label: string;
  slug: string;
};

const HORIZONTAL_SPACING = 280;
const VERTICAL_SPACING = 120;

const AgentNode = ({ data }: { data: AgentNodeData }) => {
  return (
    <>
      <Handle className="!bg-primary" position={Position.Left} type="target" />
      <div
        className={cn(
          "rounded-lg border border-l-4 border-l-chart-1 bg-card px-4 py-3 shadow-sm",
          "min-w-[180px]"
        )}
      >
        <div className="flex items-center gap-2">
          <div className="h-2 w-2 rounded-full bg-success" />
          <span className="font-medium text-sm">{data.label}</span>
        </div>
        <div className="mt-1 flex items-center gap-1.5">
          <Badge className="text-xs" variant="secondary">
            {data.slug}
          </Badge>
        </div>
      </div>
      <Handle className="!bg-primary" position={Position.Right} type="source" />
    </>
  );
};

const nodeTypes = { agent: AgentNode };

function layoutNodes(
  topologyNodes: AgentTopologyNode[],
  topologyEdges: AgentTopologyEdge[]
): { flowEdges: Edge[]; flowNodes: Node[] } {
  // Build adjacency for simple left-to-right layout.
  const hasIncoming = new Set<string>();
  const hasOutgoing = new Set<string>();

  for (const edge of topologyEdges) {
    hasOutgoing.add(edge.source);
    hasIncoming.add(edge.target);
  }

  // Assign columns: sources first, then connected, then isolated.
  const sources = topologyNodes.filter(
    (n) => hasOutgoing.has(n.agent_id) && !hasIncoming.has(n.agent_id)
  );
  const targets = topologyNodes.filter(
    (n) => hasIncoming.has(n.agent_id) && !hasOutgoing.has(n.agent_id)
  );
  const middle = topologyNodes.filter(
    (n) => hasIncoming.has(n.agent_id) && hasOutgoing.has(n.agent_id)
  );
  const isolated = topologyNodes.filter(
    (n) => !(hasIncoming.has(n.agent_id) || hasOutgoing.has(n.agent_id))
  );

  const columns = [sources, middle, targets, isolated].filter(
    (col) => col.length > 0
  );

  const flowNodes: Node[] = [];
  for (let col = 0; col < columns.length; col++) {
    const column = columns[col];
    for (let row = 0; row < column.length; row++) {
      const node = column[row];
      flowNodes.push({
        id: node.agent_id,
        type: "agent",
        position: {
          x: col * HORIZONTAL_SPACING,
          y: row * VERTICAL_SPACING,
        },
        data: {
          label: node.agent_name,
          slug: node.agent_slug,
        },
      });
    }
  }

  const flowEdges: Edge[] = topologyEdges.map((edge, index) => {
    const count = Number(edge.message_count) || 0;
    return {
      id: `e-${index}`,
      source: edge.source,
      target: edge.target,
      animated: true,
      label: String(count),
      labelStyle: { fontSize: 11, fontWeight: 500 },
      style: { strokeWidth: Math.min(1 + count / 5, 4) },
    };
  });

  return { flowEdges, flowNodes };
}

export default function AgentTopologyFlow({
  nodes: topologyNodes,
  edges: topologyEdges,
}: AgentTopologyFlowProps) {
  const { flowEdges, flowNodes } = useMemo(
    () => layoutNodes(topologyNodes, topologyEdges),
    [topologyNodes, topologyEdges]
  );

  const [nodes, , onNodesChange] = useNodesState(flowNodes);
  const [edges, , onEdgesChange] = useEdgesState(flowEdges);

  if (topologyNodes.length === 0) {
    return (
      <div className="flex h-48 items-center justify-center text-muted-foreground text-sm">
        No agents to display.
      </div>
    );
  }

  return (
    <div className="h-[400px] w-full rounded-lg border bg-card">
      <ReactFlow
        edges={edges}
        fitView
        nodes={nodes}
        nodeTypes={nodeTypes}
        onEdgesChange={onEdgesChange}
        onNodesChange={onNodesChange}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls />
      </ReactFlow>
    </div>
  );
}
