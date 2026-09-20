import assert from "node:assert/strict";
import test from "node:test";
import { applyLocalMutation, makeCommittedTree, makeWALRecord, retiredPages } from "./visualizer.js";

test("a completed mutation sequence preserves live data, WAL history, and retired pages", () => {
  let records = [];
  let tree = makeCommittedTree(records);
  const wal = [];
  let retired = [];

  for (const [operation, key, value] of [["insert", "1", "Alice"], ["insert", "2", "Bob"], ["update", "1", "Alicia"]]) {
    const nextRecords = applyLocalMutation(records, operation, key, value);
    const nextTree = makeCommittedTree(nextRecords, tree);
    wal.push(makeWALRecord({ operation, key, value }, wal.length + 1));
    retired = [...retired, ...retiredPages(tree, nextTree)];
    records = nextRecords;
    tree = nextTree;
  }

  assert.deepEqual(records, [
    { key: "1", value: "Alicia" },
    { key: "2", value: "Bob" },
  ]);
  assert.deepEqual(wal.map((entry) => entry.operation), ["insert", "insert", "update"]);
  assert.equal(wal.length, 3);
  assert.ok(retired.length >= 1);

  const afterDelete = applyLocalMutation(records, "delete", "2", "");
  assert.deepEqual(afterDelete, [{ key: "1", value: "Alicia" }]);
});


