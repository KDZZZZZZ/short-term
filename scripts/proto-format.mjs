import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import {
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const protoRoot = join(repositoryRoot, "proto");
const formattedRoot = mkdtempSync(join(tmpdir(), "short-term-proto-format-"));
const write = process.argv.includes("--write");

function protoFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const entryPath = join(directory, entry.name);
    if (entry.isDirectory()) {
      return protoFiles(entryPath);
    }
    return entry.isFile() && entry.name.endsWith(".proto") ? [entryPath] : [];
  });
}

function normalized(content) {
  return content.replaceAll("\r\n", "\n");
}

try {
  const bufBin = require.resolve("@bufbuild/buf/bin/buf");
  const result = spawnSync(
    process.execPath,
    [bufBin, "format", "proto", "--output", formattedRoot],
    { cwd: repositoryRoot, stdio: "inherit" },
  );
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }

  const changed = [];
  for (const sourcePath of protoFiles(protoRoot)) {
    const relativePath = relative(protoRoot, sourcePath);
    const formattedPath = join(formattedRoot, relativePath);
    const source = readFileSync(sourcePath, "utf8");
    const formatted = readFileSync(formattedPath, "utf8");
    if (normalized(source) === normalized(formatted)) {
      continue;
    }

    changed.push(relativePath.replaceAll("\\", "/"));
    if (write) {
      writeFileSync(sourcePath, formatted, "utf8");
    }
  }

  if (changed.length > 0) {
    const action = write ? "Formatted" : "Unformatted";
    console.error(`${action} Proto files:\n${changed.map((file) => `- ${file}`).join("\n")}`);
    if (!write) {
      process.exitCode = 1;
    }
  }
} finally {
  rmSync(formattedRoot, { recursive: true, force: true });
}
