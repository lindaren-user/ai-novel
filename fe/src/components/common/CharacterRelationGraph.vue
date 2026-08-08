<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, shallowRef, watch } from "vue";
import Graph from "graphology";
import Sigma from "sigma";

export type RelationGraphNode = {
  id: string;
  name: string;
  appearanceTime?: string;
  notes?: string;
  x?: number;
  y?: number;
};

export type RelationGraphEdge = {
  id: string;
  source: string;
  target: string;
  description?: string;
};

const props = defineProps<{
  nodes: RelationGraphNode[];
  edges: RelationGraphEdge[];
  selectedNodeId?: string | null;
  selectedEdgeId?: string | null;
  pendingDeleteEdgeId?: string | null;
  editable?: boolean;
  deletableEdges?: boolean;
}>();

const emit = defineEmits<{
  stageClick: [];
  nodeClick: [id: string];
  edgeClick: [id: string];
  edgeContextMenu: [id: string, event: MouseEvent | TouchEvent];
  deletePendingEdge: [];
  nodePositionChange: [id: string, position: { x: number; y: number }];
}>();

const container = ref<HTMLElement | null>(null);
const renderer = shallowRef<Sigma | null>(null);
const graph = shallowRef<Graph | null>(null);
const hoveredItem = ref<{ type: "node" | "edge"; id: string } | null>(null);
const draggedNodeId = ref<string | null>(null);
const draggedNodePosition = ref<{ x: number; y: number } | null>(null);
const draggedNodeOffset = ref({ x: 0, y: 0 });
const dragDistance = ref(0);
const suppressedClickNodeId = ref<string | null>(null);
const pendingDeletePosition = ref<{ x: number; y: number } | null>(null);
const graphStructureSignature = ref("");
const isDarkMode = ref(false);
let themeObserver: MutationObserver | null = null;

watch(
  () => [props.nodes, props.edges],
  () => {
    rebuildGraph();
  },
  { deep: true },
);

watch(
  () => [props.selectedNodeId, props.selectedEdgeId, props.pendingDeleteEdgeId],
  () => {
    refreshRenderer();
    updatePendingDeletePosition();
  },
);

onMounted(() => {
  syncGraphTheme();
  rebuildGraph();
  themeObserver = new MutationObserver(syncGraphTheme);
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class"],
  });
  window.addEventListener("resize", handleResize);
});

onUnmounted(() => {
  window.removeEventListener("resize", handleResize);
  themeObserver?.disconnect();
  renderer.value?.kill();
});

function rebuildGraph() {
  if (!container.value) return;

  const nextSignature = relationGraphSignature();
  const shouldResetCamera = graphStructureSignature.value !== nextSignature;
  graphStructureSignature.value = nextSignature;
  const nextGraph = new Graph({ multi: true, type: "undirected" });
  const positionedNodes = layoutNodes(props.nodes);

  for (const node of positionedNodes) {
    nextGraph.addNode(node.id, {
      ...node,
      label: node.name || "未命名人物",
      size: 20,
      color: "#e5e7eb",
    });
  }

  for (const edge of props.edges) {
    if (!nextGraph.hasNode(edge.source) || !nextGraph.hasNode(edge.target)) continue;
    nextGraph.addEdgeWithKey(edge.id, edge.source, edge.target, {
      ...edge,
      size: 2,
      color: "#d1d5db",
    });
  }

  graph.value = nextGraph;

  if (!renderer.value) {
    renderer.value = new Sigma(nextGraph, container.value, {
      allowInvalidContainer: true,
      enableEdgeEvents: true,
      renderEdgeLabels: false,
      hideEdgesOnMove: false,
      hideLabelsOnMove: false,
      doubleClickZoomingRatio: 1,
      doubleClickZoomingDuration: 0,
      labelFont: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif",
      labelSize: 13,
      labelWeight: "600",
      labelColor: { attribute: "labelColor", color: graphLabelColor() },
      defaultEdgeType: "line",
      defaultNodeColor: "#ffffff",
      defaultEdgeColor: "#d1d5db",
      minEdgeThickness: 3,
      stagePadding: 32,
      draggedEventsTolerance: 14,
      nodeReducer,
      edgeReducer,
    });
    bindRendererEvents(renderer.value);
  } else {
    renderer.value.setGraph(nextGraph);
  }

  nextTick(() => {
    renderer.value?.resize();
    if (shouldResetCamera) renderer.value?.getCamera().animatedReset({ duration: 260 });
    refreshRenderer();
    updatePendingDeletePosition();
  });
}

function relationGraphSignature() {
  const nodes = props.nodes.map((node) => node.id).join("|");
  const edges = props.edges
    .map((edge) => `${edge.id}:${edge.source}->${edge.target}`)
    .join("|");
  return `${nodes}::${edges}`;
}

function layoutNodes(nodes: RelationGraphNode[]) {
  if (nodes.length === 1) {
    const [node] = nodes;
    return hasGraphPosition(node) ? [node] : [{ ...node, x: 0, y: 0 }];
  }

  const positioned = new Map<string, RelationGraphNode>();
  for (const node of nodes) {
    if (hasGraphPosition(node)) {
      positioned.set(node.id, node);
    }
  }

  const unpositioned = nodes.filter((node) => !hasGraphPosition(node));
  const columns = Math.ceil(Math.sqrt(unpositioned.length));
  const rows = Math.ceil(unpositioned.length / columns);
  const gap = 3.2;
  for (const [index, node] of unpositioned.entries()) {
    const row = Math.floor(index / columns);
    const column = index % columns;
    positioned.set(node.id, {
      ...node,
      x: (column - (columns - 1) / 2) * gap + (row % 2 ? gap * 0.28 : 0),
      y: (row - (rows - 1) / 2) * gap,
    });
  }

  return nodes.map((node) => positioned.get(node.id) || node);
}

function hasGraphPosition(node: RelationGraphNode) {
  if (typeof node.x !== "number" || typeof node.y !== "number") return false;
  // 旧 SVG 布局传入的是数百像素坐标；Sigma 使用图坐标，只保留组件拖拽后产生的小范围坐标。
  return Math.abs(node.x) <= 30 && Math.abs(node.y) <= 30;
}

function bindRendererEvents(instance: Sigma) {
  instance.on("clickStage", () => {
    if (suppressedClickNodeId.value) return;
    pendingDeletePosition.value = null;
    emit("stageClick");
  });
  instance.on("clickNode", ({ node }) => {
    if (suppressedClickNodeId.value === node) {
      suppressedClickNodeId.value = null;
      return;
    }
    emit("nodeClick", node);
  });
  instance.on("clickEdge", ({ edge }) => {
    emit("edgeClick", edge);
  });
  instance.on("rightClickEdge", ({ edge, event }) => {
    if (!props.deletableEdges) return;
    event.preventSigmaDefault();
    emit("edgeContextMenu", edge, event.original);
  });
  instance.on("enterNode", ({ node }) => {
    setGraphCursor("pointer");
    hoveredItem.value = { type: "node", id: node };
    refreshRenderer();
  });
  instance.on("leaveNode", () => {
    setGraphCursor("grab");
    hoveredItem.value = null;
    refreshRenderer();
  });
  instance.on("enterEdge", ({ edge }) => {
    setGraphCursor("pointer");
    hoveredItem.value = { type: "edge", id: edge };
    refreshRenderer();
  });
  instance.on("leaveEdge", () => {
    setGraphCursor("grab");
    hoveredItem.value = null;
    refreshRenderer();
  });
  instance.on("downNode", ({ node, event }) => {
    if (!props.editable) return;
    draggedNodeId.value = node;
    draggedNodePosition.value = null;
    dragDistance.value = 0;
    const pointerPosition = instance.viewportToGraph({ x: event.x, y: event.y });
    const nodeAttrs = graph.value?.getNodeAttributes(node);
    draggedNodeOffset.value = {
      x: Number(nodeAttrs?.x || 0) - pointerPosition.x,
      y: Number(nodeAttrs?.y || 0) - pointerPosition.y,
    };
    instance.getCamera().disable();
    event.preventSigmaDefault();
  });

  instance.getMouseCaptor().on("mousemovebody", (event) => {
    if (!draggedNodeId.value) return;
    const pointerPosition = instance.viewportToGraph({ x: event.x, y: event.y });
    const position = {
      x: pointerPosition.x + draggedNodeOffset.value.x,
      y: pointerPosition.y + draggedNodeOffset.value.y,
    };
    const current = graph.value?.getNodeAttributes(draggedNodeId.value);
    if (current) {
      dragDistance.value += Math.hypot(position.x - Number(current.x), position.y - Number(current.y));
    }
    graph.value?.setNodeAttribute(draggedNodeId.value, "x", position.x);
    graph.value?.setNodeAttribute(draggedNodeId.value, "y", position.y);
    draggedNodePosition.value = position;
    instance.refresh();
    updatePendingDeletePosition();
  });

  instance.getMouseCaptor().on("rightClick", (event) => {
    if (!(event.original instanceof MouseEvent)) return;
    handleGraphRightClick(event.x, event.y, event.original);
  });

  instance.getMouseCaptor().on("mouseup", () => {
    if (draggedNodeId.value && draggedNodePosition.value) {
      emit("nodePositionChange", draggedNodeId.value, draggedNodePosition.value);
    }
    if (draggedNodeId.value && dragDistance.value > 0.09) {
      suppressedClickNodeId.value = draggedNodeId.value;
      window.setTimeout(() => {
        suppressedClickNodeId.value = null;
      }, 0);
    }
    draggedNodeId.value = null;
    draggedNodePosition.value = null;
    draggedNodeOffset.value = { x: 0, y: 0 };
    dragDistance.value = 0;
    instance.getCamera().enable();
  });

  instance.getCamera().on("updated", updatePendingDeletePosition);
}

function nodeReducer(node: string, data: Record<string, unknown>) {
  const selected = props.selectedNodeId === node;
  const related =
    selected ||
    (!!props.selectedEdgeId &&
      graph.value?.extremities(props.selectedEdgeId).includes(node));
  const hovered = hoveredItem.value?.type === "node" && hoveredItem.value.id === node;
  const selectedColor = isDarkMode.value ? "#334155" : "#475569";
  const hoverColor = isDarkMode.value ? "#475569" : "#64748b";
  const baseColor = isDarkMode.value ? "#1f2937" : "#94a3b8";
  return {
    ...data,
    color: selected ? selectedColor : hovered || related ? hoverColor : baseColor,
    labelColor: hovered ? "#111827" : graphLabelColor(),
    size: selected ? 24 : hovered ? 22 : 20,
    highlighted: false,
    zIndex: selected || hovered ? 10 : 1,
  };
}

function edgeReducer(edge: string, data: Record<string, unknown>) {
  const selected = props.selectedEdgeId === edge || props.pendingDeleteEdgeId === edge;
  const hovered = hoveredItem.value?.type === "edge" && hoveredItem.value.id === edge;
  const selectedColor = isDarkMode.value ? "#f9fafb" : "#111827";
  const hoverColor = isDarkMode.value ? "#9ca3af" : "#6b7280";
  const baseColor = isDarkMode.value ? "#6b7280" : "#d1d5db";
  return {
    ...data,
    color: selected ? selectedColor : hovered ? hoverColor : baseColor,
    size: selected || hovered ? 4 : 2,
    zIndex: selected || hovered ? 8 : 0,
  };
}

function refreshRenderer() {
  renderer.value?.refresh();
}

function handleResize() {
  renderer.value?.resize();
  updatePendingDeletePosition();
}

function syncGraphTheme() {
  isDarkMode.value = document.documentElement.classList.contains("dark");
  renderer.value?.setSetting("labelColor", { attribute: "labelColor", color: graphLabelColor() });
  refreshRenderer();
}

function graphLabelColor() {
  return isDarkMode.value ? "#f9fafb" : "#111827";
}

function setGraphCursor(cursor: string) {
  if (container.value) container.value.style.cursor = cursor;
}

function edgeMidpointViewport(edgeId: string) {
  const currentGraph = graph.value;
  const currentRenderer = renderer.value;
  if (!currentGraph || !currentRenderer || !currentGraph.hasEdge(edgeId)) return null;
  const [source, target] = currentGraph.extremities(edgeId);
  const sourceAttrs = currentGraph.getNodeAttributes(source);
  const targetAttrs = currentGraph.getNodeAttributes(target);
  return currentRenderer.graphToViewport({
    x: (Number(sourceAttrs.x) + Number(targetAttrs.x)) / 2,
    y: (Number(sourceAttrs.y) + Number(targetAttrs.y)) / 2,
  });
}

function updatePendingDeletePosition() {
  const edgeId = props.pendingDeleteEdgeId;
  if (!edgeId) {
    pendingDeletePosition.value = null;
    return;
  }
  const position = edgeMidpointViewport(edgeId);
  pendingDeletePosition.value = position ? { x: position.x + 12, y: position.y - 34 } : null;
}

function edgeIdNearPointer(x: number, y: number) {
  let nearest: { id: string; distance: number } | null = null;
  for (const edge of props.edges) {
    const distance = edgeDistanceToPointer(edge.id, x, y);
    if (distance === null || distance > 14) continue;
    if (!nearest || distance < nearest.distance) nearest = { id: edge.id, distance };
  }
  return nearest?.id || null;
}

function edgeDistanceToPointer(edgeId: string, x: number, y: number) {
  const currentGraph = graph.value;
  const currentRenderer = renderer.value;
  if (!currentGraph || !currentRenderer || !currentGraph.hasEdge(edgeId)) return null;
  const [source, target] = currentGraph.extremities(edgeId);
  const sourceAttrs = currentGraph.getNodeAttributes(source);
  const targetAttrs = currentGraph.getNodeAttributes(target);
  const sourcePosition = currentRenderer.graphToViewport({ x: Number(sourceAttrs.x), y: Number(sourceAttrs.y) });
  const targetPosition = currentRenderer.graphToViewport({ x: Number(targetAttrs.x), y: Number(targetAttrs.y) });
  return pointToSegmentDistance(
    x,
    y,
    sourcePosition.x,
    sourcePosition.y,
    targetPosition.x,
    targetPosition.y,
  );
}

function pointToSegmentDistance(px: number, py: number, x1: number, y1: number, x2: number, y2: number) {
  const dx = x2 - x1;
  const dy = y2 - y1;
  if (dx === 0 && dy === 0) return Math.hypot(px - x1, py - y1);
  const ratio = Math.max(0, Math.min(1, ((px - x1) * dx + (py - y1) * dy) / (dx * dx + dy * dy)));
  return Math.hypot(px - (x1 + ratio * dx), py - (y1 + ratio * dy));
}

function handleGraphRightClick(x: number, y: number, event: MouseEvent) {
  if (!props.deletableEdges) return;
  const edgeId = edgeIdNearPointer(x, y);
  if (!edgeId) return;
  event.preventDefault();
  emit("edgeContextMenu", edgeId, event);
}
</script>

<template>
  <div class="relative h-full min-h-[360px] overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-950">
    <div
      ref="container"
      class="h-full min-h-[360px] cursor-grab bg-[radial-gradient(circle_at_1px_1px,rgba(17,24,39,0.08)_1px,transparent_0)] [background-size:24px_24px] active:cursor-grabbing dark:bg-[radial-gradient(circle_at_1px_1px,rgba(255,255,255,0.08)_1px,transparent_0)]"
    />
    <div class="pointer-events-none absolute left-3 top-3 rounded-full border border-gray-200 bg-white/85 px-3 py-1 text-xs text-gray-500 shadow-sm backdrop-blur dark:border-gray-700 dark:bg-gray-900/85 dark:text-gray-400">
      滚轮缩放 · 拖动画布 · 点击查看详情<span v-if="deletableEdges"> · 右键删除关系</span>
    </div>
    <button
      v-if="pendingDeletePosition"
      class="absolute z-30 flex size-8 items-center justify-center rounded-lg border border-red-200 bg-white text-red-500 shadow-lg transition-colors hover:bg-red-50 dark:border-red-900 dark:bg-gray-900 dark:hover:bg-red-950/40"
      :style="{ left: `${pendingDeletePosition.x}px`, top: `${pendingDeletePosition.y}px` }"
      @click.stop="emit('deletePendingEdge')"
    >
      <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M3 6h18" />
        <path d="M8 6V4h8v2" />
        <path d="M10 11v6" />
        <path d="M14 11v6" />
        <path d="M6 6l1 15h10l1-15" />
      </svg>
    </button>
  </div>
</template>
