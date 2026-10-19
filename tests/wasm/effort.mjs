// Compares search-effort presets: time and final error for the same shape count.
//   node tests/wasm/effort.mjs
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
await import(pathToFileURL(path.join(root, "web", "wasm_exec.js")).href);
const go = new Go();
const { instance } = await WebAssembly.instantiate(fs.readFileSync(path.join(root, "web", "primitive.wasm")), go.importObject);
go.run(instance);
for (let i = 0; i < 100 && !globalThis.primitive; i++) await new Promise((r) => setTimeout(r, 20));
const P = globalThis.primitive;
const img = new Uint8Array(fs.readFileSync(path.join(root, "examples", process.argv[2] || "monalisa.png")));
const SIZE = Number(process.argv[3] || 256);
const SHAPES = Number(process.argv[4] || 40);

console.log(`image ${process.argv[2] || "monalisa.png"}, working size ${SIZE}, ${SHAPES} shapes, one run each`);
for (const [name, cand, rest] of [["best 1000/16", 1000, 16], ["balanced 300/6", 300, 6], ["fast 100/3", 100, 3], ["draft 40/2", 40, 2]]) {
  P.init(img, SIZE, 256, "", 0, cand, rest);
  const t = performance.now();
  let r;
  for (let i = 0; i < SHAPES; i++) r = P.step(1, 128);
  const ms = performance.now() - t;
  console.log(`${name.padEnd(16)} ${(ms / SHAPES).toFixed(0).padStart(5)} ms/shape  total ${(ms / 1000).toFixed(1).padStart(5)} s  final error ${r.score.toFixed(4)}`);
}
process.exit(0);
