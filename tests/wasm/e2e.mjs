// End-to-end test of the browser build, run in Node (no browser needed).
//   make web   (or: GOOS=js GOARCH=wasm go build -o web/primitive.wasm ./cmd/wasm)
//   node tests/wasm/e2e.mjs
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
await import(pathToFileURL(path.join(root, "web", "wasm_exec.js")).href);

const go = new Go();
const { instance } = await WebAssembly.instantiate(
  fs.readFileSync(path.join(root, "web", "primitive.wasm")),
  go.importObject,
);
go.run(instance); // never resolves, the Go side waits for calls
for (let i = 0; i < 100 && !globalThis.primitive; i++) await new Promise((r) => setTimeout(r, 20));
const P = globalThis.primitive;
assert.ok(P, "global primitive object was not registered");

const owl = new Uint8Array(fs.readFileSync(path.join(root, "examples", "owl.png")));
const fills = (svg) => [...svg.matchAll(/fill="(#[0-9a-f]{6})"/g)].map((m) => m[1]);
let passed = 0;
const test = (name, fn) => {
  fn();
  passed++;
  console.log("ok  " + name);
};

test("rejects bad image bytes", () => {
  const r = P.init(new Uint8Array([1, 2, 3]), 128, 256, "", 0);
  assert.match(r.error, /could not read image/);
});

test("rejects bad palette", () => {
  const r = P.init(owl, 128, 256, "zzz", 0);
  assert.match(r.error, /invalid palette color/);
});

test("init returns output size", () => {
  const r = P.init(owl, 128, 256, "", 0);
  assert.equal(r.error, undefined);
  assert.equal(r.width, 256);
  assert.equal(r.height, 256);
});

test("step adds shapes and lowers the score", () => {
  const a = P.step(1, 128);
  let b;
  for (let i = 0; i < 9; i++) b = P.step(1, 128);
  assert.equal(a.shapes, 1);
  assert.equal(b.shapes, 10);
  assert.ok(b.score < a.score, `score should drop: ${a.score} -> ${b.score}`);
});

test("pixels are RGBA of the right size and not flat", () => {
  const px = P.pixels();
  assert.equal(px.length, 256 * 256 * 4);
  const distinct = new Set();
  for (let i = 0; i < px.length; i += 4000) distinct.add(px[i] + "," + px[i + 1] + "," + px[i + 2]);
  assert.ok(distinct.size > 3, "image looks flat");
});

test("svg export, static and animated", () => {
  const plain = P.svg(0, 0);
  assert.equal((plain.match(/<polygon/g) || []).length, 10);
  assert.match(plain, /viewBox="0 0 128 128"/);
  assert.ok(!plain.includes("animation-delay"));
  const anim = P.svg(0.1, 0);
  assert.match(anim, /animation-delay:0\.900s/);
  assert.ok(P.svg(0, 2).includes('stdDeviation="2"'));
});

test("png export has the PNG signature", () => {
  const png = P.png();
  assert.deepEqual([...png.slice(0, 4)], [0x89, 0x50, 0x4e, 0x47]);
  assert.ok(png.length > 1000);
});

test("palette restricts colors", () => {
  const pal = ["#101010", "#e8d9a0"];
  assert.equal(P.init(owl, 128, 256, pal.join(","), 0).error, undefined);
  for (let i = 0; i < 8; i++) P.step(1, 128);
  const used = fills(P.svg(0, 0)).slice(1); // first fill is the background
  assert.equal(used.length, 8);
  for (const c of used) assert.ok(pal.includes(c), `${c} is not in the palette`);
});

test("a new init forgets the old palette and shapes", () => {
  P.init(owl, 128, 256, "", 0);
  const r = P.step(1, 128);
  assert.equal(r.shapes, 1);
  for (let i = 0; i < 7; i++) P.step(1, 128);
  const used = new Set(fills(P.svg(0, 0)).slice(1));
  assert.ok(used.size > 2, "palette leaked into the next run");
});

test("every shape type and the edge option run", () => {
  for (const t of [1, 2, 3, 4, 5, 6, 7, 8, 0]) {
    assert.equal(P.init(owl, 128, 256, "", 0.6).error, undefined);
    assert.equal(P.step(t, 128).shapes, 1, `shape type ${t}`);
  }
});

console.log(`\n${passed} passed`);
process.exit(0); // the Go runtime keeps the event loop busy otherwise
