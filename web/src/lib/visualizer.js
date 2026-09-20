export const structureLabels = {
  map: "Hash map",
  "sorted-slice": "Sorted array",
  "btree-memory": "B+Tree in memory",
  "btree-durable": "B+Tree on disk",
};

export const eventLabels = {
  operation_selected: "Operation started",
  lookup: "Lookup",
  comparison: "Compare key",
  traversal: "Traverse node",
  mutation: "Update entry",
  result_delivered: "Result returned",
  cache_hit: "Cache hit",
  cache_miss: "Cache miss",
  cache_fill: "Cache fill",
  cache_invalidation: "Cache invalidation",
  cache_eviction: "Cache eviction",
  cache_bypass: "Cache bypass",
};

export function displayStructure(name) {
  return structureLabels[name] || name || "No structure selected";
}

export function displayEvent(event) {
  return eventLabels[event?.type] || event?.type || "Waiting";
}

export function makeBTree(records = []) {
  const sorted = [...records].sort((a, b) => String(a.key).localeCompare(String(b.key)));
  const leaves = [];
  for (let index = 0; index < sorted.length; index += 4) {
    const entries = sorted.slice(index, index + 4);
    leaves.push({ id: `leaf-${Math.floor(index / 4) + 1}`, kind: "leaf", entries });
  }
  if (!leaves.length) leaves.push({ id: "leaf-1", kind: "leaf", entries: [] });
  return {
    root: { id: "root-1", kind: "internal", entries: leaves.slice(1).map((leaf) => ({ key: leaf.entries[0]?.key || "", value: `→ ${leaf.id}` })) },
    leaves,
  };
}

export function activeNode(traceEvent) {
  if (!traceEvent) return "";
  if (traceEvent.key) return `key:${traceEvent.key}`;
  if (traceEvent.node_id) return `node-${traceEvent.node_id}`;
  if (traceEvent.page_id) return `page-${traceEvent.page_id}`;
  return "";
}

export function resultSummary(result) {
  const item = result?.results?.[result.results.length - 1];
  if (!item) return "No operation yet";
  if (item.status === "found") return `Found ${item.key || "record"}`;
  if (item.status === "not_found") return "Key not found";
  if (item.status === "stored") return `Stored ${item.key}`;
  if (item.status === "deleted") return `Deleted ${item.key}`;
  if (item.status === "empty") return "No matching records";
  if (item.status === "matched") return `${item.count} matching records`;
  return item.status || "Completed";
}
