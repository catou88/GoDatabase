import assert from "node:assert/strict";
import test from "node:test";
import { applyLocalMutation, makeCommittedTree, makeWALRecord, mutationChanges, retiredPages, shouldShowCommittedLayers, targetForTransition } from "./visualizer.js";

const seed = [
  { key: "1", value: "Alice" },
  { key: "2", value: "Bob" },
];

test("inserting preserves existing records and adds one record", () => {
  assert.deepEqual(applyLocalMutation(seed, "insert", "3", "Carol"), [
    ...seed,
    { key: "3", value: "Carol" },
  ]);
});

test("insert and update replace only the selected key", () => {
  const inserted = applyLocalMutation(seed, "insert", "2", "Bobby");
  const updated = applyLocalMutation(inserted, "update", "1", "Alicia");
  assert.deepEqual(updated, [
    { key: "1", value: "Alicia" },
    { key: "2", value: "Bobby" },
  ]);
});

test("delete preserves unrelated records", () => {
  assert.deepEqual(applyLocalMutation(seed, "delete", "1", ""), [{ key: "2", value: "Bob" }]);
});

test("read and range are observational", () => {
  assert.deepEqual(applyLocalMutation(seed, "read", "1", "ignored"), seed);
  assert.deepEqual(applyLocalMutation(seed, "range", "1", "ignored"), seed);
});

test("updating a missing key does not add a record or mutate the tree", () => {
  assert.equal(mutationChanges(seed, "update", "9", "New"), false);
  assert.deepEqual(applyLocalMutation(seed, "update", "9", "New"), seed);
});

test("each transition highlights its destination component", () => {
  assert.deepEqual([0, 1, 2, 3, 4, 5].map(targetForTransition), [
    "logical-table",
    "wal",
    "buffer-pool",
    "btree-node",
    "disk",
    null,
  ]);
});

test("completed state remains visible after the drawer closes", () => {
  assert.equal(shouldShowCommittedLayers({ stepsComplete: false, pathIndex: -1, recordCount: 2, walCount: 4 }), true);
  assert.equal(shouldShowCommittedLayers({ stepsComplete: false, pathIndex: 0, recordCount: 2, walCount: 4 }), true);
  assert.equal(shouldShowCommittedLayers({ stepsComplete: false, pathIndex: -1, recordCount: 0, walCount: 0 }), false);
  assert.equal(shouldShowCommittedLayers({ stepsComplete: true, pathIndex: -1, recordCount: 0, walCount: 1 }), true);
});

test("WAL records are append-only and sequenced by the caller", () => {
  assert.deepEqual(makeWALRecord({ operation: "insert", key: "1", value: "Alice" }, 1), {
    id: 1,
    operation: "insert",
    key: "1",
    value: "Alice",
  });
  assert.deepEqual(makeWALRecord({ operation: "delete", key: "1", value: "Alice" }, 2), {
    id: 2,
    operation: "delete",
    key: "1",
    value: "",
  });
});

test("unchanged pages keep their IDs while changed pages are retired", () => {
  const first = makeCommittedTree([
    ...seed,
    { key: "3", value: "Carol" },
    { key: "4", value: "Diana" },
    { key: "5", value: "Eve" },
  ]);
  const next = makeCommittedTree([
    ...seed,
    { key: "3", value: "Caroline" },
    { key: "4", value: "Diana" },
    { key: "5", value: "Eve" },
  ], first);
  assert.equal(next.leaves[1].id, first.leaves[1].id);
  assert.ok(retiredPages(first, next).some((page) => page.id === first.leaves[0].id));
});
