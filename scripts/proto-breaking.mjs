import { spawnSync } from "node:child_process";

const base = process.env.PROTO_BREAKING_BASE || "origin/main";
const tree = spawnSync(
  "git",
  ["ls-tree", "-r", "--name-only", base, "--", "proto"],
  { encoding: "utf8" },
);
if (tree.status !== 0) {
  process.stderr.write(tree.stderr || `unable to inspect Proto baseline ${base}\n`);
  process.exit(tree.status ?? 1);
}

const hasProtoBaseline = tree.stdout
  .split("\n")
  .some((path) => path.endsWith(".proto"));
if (!hasProtoBaseline) {
  console.log(`Proto breaking check skipped: ${base} has no .proto baseline yet.`);
  process.exit(0);
}

const result = spawnSync(
  "buf",
  ["breaking", "--against", `.git#branch=${base}`],
  { stdio: "inherit" },
);
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
process.exit(result.status ?? 1);
