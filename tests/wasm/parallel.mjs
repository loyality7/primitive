// Runs the multi-thread protocol of the web page with real threads:
// every worker holds a copy of the picture, all search at once, the best
// shape is applied to all copies. Checks that the copies stay identical and
// prints the speed-up.
//   node tests/wasm/parallel.mjs [workers] [shapes] [size]
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Worker } from "node:worker_threads";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const photo = new Uint8Array(fs.readFileSync(path.join(root, "examples", "monalisa.png")));
const WORKERS = Number(process.argv[2] || Math.min(4, os.availableParallelism()));
const SHAPES = Number(process.argv[3] || 30);
const SIZE = Number(process.argv[4] || 192);
const CAND = 300, RESTARTS = 6; // the "Balanced" preset of the page

class Instance {
  constructor() {
    this.w = new Worker(new URL("./instance-worker.mjs", import.meta.url));
    this.next = 1;
    this.pending = new Map();
    this.ready = new Promise((r) => this.pending.set(0, r));
    this.w.on("message", ({ id, result }) => { this.pending.get(id)(result); this.pending.delete(id); });
  }
  call(fn, ...args) {
    const id = this.next++;
    return new Promise((r) => { this.pending.set(id, r); this.w.postMessage({ id, fn, args }); });
  }
}

async function run(n) {
  const pool = Array.from({ length: n }, () => new Instance());
  await Promise.all(pool.map((p) => p.ready));
  const inits = await Promise.all(pool.map((p, i) => p.call("init", photo, SIZE, 256, "", 0, CAND, RESTARTS, 1000 + i)));
  inits.forEach((r) => assert.equal(r.error, undefined, r.error));
  const each = Math.ceil(RESTARTS / n);
  const t = performance.now();
  let scores;
  for (let step = 0; step < SHAPES; step++) {
    const found = await Promise.all(pool.map((p) => p.call("search", 0, 128, each)));
    const best = found.reduce((a, b) => (b.energy < a.energy ? b : a));
    scores = await Promise.all(pool.map((p) => p.call("apply", best.type, best.params, best.alpha)));
    for (const s of scores) {
      assert.equal(s.error, undefined);
      assert.equal(s.shapes, step + 1);
      assert.equal(s.score, scores[0].score, `copies drifted apart at shape ${step + 1}`);
    }
  }
  const ms = performance.now() - t;
  const svgs = await Promise.all(pool.map((p) => p.call("svg", 0, 0)));
  for (const s of svgs) assert.equal(s, svgs[0], "the copies drew different pictures");
  pool.forEach((p) => p.w.terminate());
  return { ms, score: scores[0].score };
}

console.log(`${SHAPES} shapes, working size ${SIZE}, ${os.availableParallelism()} cores available`);
const one = await run(1);
console.log(`1 thread : ${(one.ms / SHAPES).toFixed(0).padStart(5)} ms per shape, final error ${one.score.toFixed(4)}`);
if (WORKERS > 1) {
  const many = await run(WORKERS);
  console.log(`${WORKERS} threads: ${(many.ms / SHAPES).toFixed(0).padStart(5)} ms per shape, final error ${many.score.toFixed(4)}  ->  ${(one.ms / many.ms).toFixed(1)}x faster`);
  assert.ok(many.score < one.score * 1.25, "searching in parallel should not be much worse");
}
console.log("copies stayed identical at every step");
process.exit(0);
