// One engine instance on its own thread, driven by messages. Used by parallel.mjs.
import { parentPort } from "node:worker_threads";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
await import(pathToFileURL(path.join(root, "web", "wasm_exec.js")).href);
const go = new Go();
const { instance } = await WebAssembly.instantiate(fs.readFileSync(path.join(root, "web", "primitive.wasm")), go.importObject);
go.run(instance);
for (let i = 0; i < 100 && !globalThis.primitive; i++) await new Promise((r) => setTimeout(r, 20));

parentPort.on("message", ({ id, fn, args }) => {
  parentPort.postMessage({ id, result: globalThis.primitive[fn](...args) });
});
parentPort.postMessage({ id: 0, result: "ready" });
