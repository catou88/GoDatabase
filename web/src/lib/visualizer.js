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

export const transitionTargets = ["logical-table", "wal", "buffer-pool", "btree-node", "disk", null];

export function targetForTransition(stepIndex) {
  return transitionTargets[stepIndex] || null;
}

export function shouldShowCommittedLayers({ stepsComplete, pathIndex, recordCount, walCount }) {
  return stepsComplete || recordCount > 0 || walCount > 0;
}

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
    leaves.push({ id: `page-${Math.floor(index / 4) + 1}`, kind: "leaf", entries });
  }
  if (!leaves.length) leaves.push({ id: "page-1", kind: "leaf", entries: [] });
  return {
    root: { id: "root-1", kind: "internal", entries: leaves.slice(1).map((leaf) => ({ key: leaf.entries[0]?.key || "", value: `→ ${leaf.id}` })) },
    leaves,
  };
}

function sameEntries(left, right) {
  return left.length === right.length && left.every((entry, index) => String(entry.key) === String(right[index].key) && String(entry.value) === String(right[index].value));
}

export function makeCommittedTree(records = [], previousTree = null) {
  const sorted = [...records].sort((a, b) => String(a.key).localeCompare(String(b.key)));
  const groups = [];
  for (let index = 0; index < sorted.length; index += 4) groups.push(sorted.slice(index, index + 4));
  if (!groups.length) groups.push([]);

  let nextPage = previousTree?.nextPage || 1;
  const usedPages = new Set();
  const leaves = groups.map((entries) => {
    const previous = previousTree?.leaves.find((leaf) => !usedPages.has(leaf.id) && sameEntries(leaf.entries, entries));
    if (previous) {
      usedPages.add(previous.id);
      return { id: previous.id, kind: "leaf", entries };
    }
    const leaf = { id: `page-${nextPage}`, kind: "leaf", entries };
    nextPage += 1;
    return leaf;
  });
  const rootEntries = leaves.slice(1).map((leaf) => ({ key: leaf.entries[0]?.key || "", value: `→ ${leaf.id}` }));
  const previousRoot = previousTree?.root;
  const sameChildren = previousTree?.leaves.map((leaf) => leaf.id).join("|") === leaves.map((leaf) => leaf.id).join("|");
  const root = previousRoot && sameChildren && sameEntries(previousRoot.entries, rootEntries)
    ? { ...previousRoot, entries: rootEntries }
    : { id: `root-${nextPage++}`, kind: "internal", entries: rootEntries };
  return { root, leaves, nextPage };
}

export function applyLocalMutation(records, operation, key, value) {
  const targetKey = String(key);
  const current = records.map((record) => ({ key: String(record.key), value: String(record.value) }));
  if (operation === "delete") return current.filter((record) => record.key !== targetKey);
  if (operation === "update" && !current.some((record) => record.key === targetKey)) return current;
  if (operation !== "insert" && operation !== "update") return current;

  const nextRecord = { key: targetKey, value: String(value) };
  const existingIndex = current.findIndex((record) => record.key === targetKey);
  if (existingIndex === -1) return [...current, nextRecord];
  return current.map((record, index) => index === existingIndex ? nextRecord : record);
}

export function mutationChanges(records, operation, key, value) {
  const targetKey = String(key);
  const existing = records.find((record) => String(record.key) === targetKey);
  if (operation === "update") return Boolean(existing && String(existing.value) !== String(value));
  if (operation === "delete") return Boolean(existing);
  if (operation === "insert") return !existing || String(existing.value) !== String(value);
  return false;
}

export function makeWALRecord(operation, id) {
  return { id, operation: operation.operation, key: operation.key, value: operation.operation === "delete" ? "" : operation.value };
}

export function retiredPages(previousTree, nextTree) {
  if (!previousTree) return [];
  const livePageIDs = new Set([nextTree.root.id, ...nextTree.leaves.map((leaf) => leaf.id)]);
  const pages = previousTree.leaves.filter((leaf) => !livePageIDs.has(leaf.id) && leaf.entries.length > 0);
  if (!livePageIDs.has(previousTree.root.id) && (previousTree.root.entries.length > 0 || previousTree.leaves.some((leaf) => leaf.entries.length > 0))) pages.push(previousTree.root);
  return pages;
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
