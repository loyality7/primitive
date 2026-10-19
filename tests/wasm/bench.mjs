// Prints the time per step of the browser build at a few working sizes.
//   node tests/wasm/bench.mjs
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
const owl = new Uint8Array(fs.readFileSync(path.join(root, "examples", "owl.png")));

for (const size of [128, 256, 512]) {
  P.init(owl, size, 512, "", 0);
  const t = performance.now();
  const steps = 5;
  for (let i = 0; i < steps; i++) P.step(1, 128);
  console.log(`working size ${size}: ${((performance.now() - t) / steps).toFixed(0)} ms per shape`);
}
process.exit(0);
