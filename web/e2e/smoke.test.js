import assert from "node:assert/strict";
import test from "node:test";

const baseURL = process.env.NEXT_E2E_URL;

test("Next frontend serves the debugger shell without trace toggle controls", { skip: !baseURL }, async () => {
  const response = await fetch(`${baseURL}/`);
  assert.equal(response.status, 200);
  const html = await response.text();
  assert.match(html, /AlgoDB Lab/);
  assert.match(html, /B\+ TREE/);
  assert.match(html, /persistent-visualizer-v2/);
  assert.match(html, /logical-records/);
  assert.doesNotMatch(html, /trace execution/);
});
