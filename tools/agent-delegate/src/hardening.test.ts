import { expect, test } from "bun:test";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

test("write capability blocks without approval", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      policy: { default_capabilities: ["read"], approval_required_for: ["write"], allow_heuristic_fallback: true },
      adapters: {
        codex: {
          enabled: true,
          command: "echo",
          args: ["ok"],
          timeout_sec: 5,
          supports_guarded_execution: true,
          supported_capabilities: ["read", "write"],
        },
      },
    }),
  );

  const proc = Bun.spawn(["bun", "run", "src/index.ts", "run", "--request", "-", "--json", "--config", configPath], {
    cwd: process.cwd(),
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  proc.stdin.write(JSON.stringify({ request: { adapter: "codex", task: "Update note.txt", capabilities: ["write"] }, approval_granted: false }));
  proc.stdin.end();

  const payload = JSON.parse(await new Response(proc.stdout).text());
  expect(payload.status).toBe("blocked");
  expect(payload.policy.capabilities_requested).toEqual(["write"]);
});

test("unsupported adapter capabilities fail clearly", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        gemini: {
          enabled: true,
          command: "echo",
          args: ["ok"],
          timeout_sec: 5,
          supports_guarded_execution: true,
          supported_capabilities: ["read"],
        },
      },
    }),
  );

  const proc = Bun.spawn(["bun", "run", "src/index.ts", "run", "--request", "-", "--json", "--config", configPath], {
    cwd: process.cwd(),
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  proc.stdin.write(JSON.stringify({ request: { adapter: "gemini", task: "Execute a tool", capabilities: ["exec"] }, approval_granted: true }));
  proc.stdin.end();

  const payload = JSON.parse(await new Response(proc.stdout).text());
  expect(payload.status).toBe("blocked");
  expect(payload.risk.reason).toContain("does not support capabilities");
});

test("allowed_paths constrains context access and change tracking", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const projectDir = path.join(tempDir, "project");
  await mkdir(path.join(projectDir, "safe"), { recursive: true });
  await mkdir(path.join(projectDir, "other"), { recursive: true });
  await writeFile(path.join(projectDir, "safe", "note.txt"), "before\n");
  await writeFile(path.join(projectDir, "other", "skip.txt"), "keep\n");

  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
printf "after\\n" > safe/note.txt
printf "changed\\n" > other/skip.txt
echo "done"
`,
    { mode: 0o755 },
  );

  const configPath = path.join(tempDir, "agent-delegate.json");
  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        gemini: {
          enabled: true,
          command: adapterPath,
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          supported_capabilities: ["read", "write"],
        },
      },
    }),
  );

  const proc = Bun.spawn(["bun", "run", "src/index.ts", "run", "--request", "-", "--json", "--config", configPath], {
    cwd: process.cwd(),
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  proc.stdin.write(
    JSON.stringify({
      request: {
        adapter: "gemini",
        task: "Update allowed file only",
        cwd: projectDir,
        capabilities: ["write"],
        allowed_paths: ["safe"],
        context: [{ type: "file", path: "safe/note.txt" }],
      },
      approval_granted: true,
    }),
  );
  proc.stdin.end();

  const payload = JSON.parse(await new Response(proc.stdout).text());
  expect(payload.status).toBe("completed");
  expect(payload.changes.created).toEqual([]);
  expect(payload.changes.updated).toEqual(["safe/note.txt"]);
  expect(payload.changes.deleted).toEqual([]);
  expect(payload.artifacts).toHaveLength(1);
});

test("change tracking reports deleted files", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const projectDir = path.join(tempDir, "project");
  await mkdir(projectDir, { recursive: true });
  await writeFile(path.join(projectDir, "note.txt"), "before\n");

  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
rm note.txt
echo "done"
`,
    { mode: 0o755 },
  );

  const configPath = path.join(tempDir, "agent-delegate.json");
  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        gemini: {
          enabled: true,
          command: adapterPath,
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          supported_capabilities: ["read", "write"],
        },
      },
    }),
  );

  const proc = Bun.spawn(["bun", "run", "src/index.ts", "run", "--request", "-", "--json", "--config", configPath], {
    cwd: process.cwd(),
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  proc.stdin.write(
    JSON.stringify({
      request: {
        adapter: "gemini",
        task: "Delete note.txt",
        cwd: projectDir,
        capabilities: ["write"],
        context: [{ type: "file", path: "note.txt" }],
      },
      approval_granted: true,
    }),
  );
  proc.stdin.end();

  const payload = JSON.parse(await new Response(proc.stdout).text());
  expect(payload.status).toBe("completed");
  expect(payload.changes.deleted).toEqual(["note.txt"]);
  expect(payload.artifacts[0].kind).toBe("deleted_file");
});

test("structured output validates against schema", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
echo '{"summary":"ok","files":["a.ts"]}'
`,
    { mode: 0o755 },
  );

  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        gemini: {
          enabled: true,
          command: adapterPath,
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          supported_capabilities: ["read"],
        },
      },
    }),
  );

  const proc = Bun.spawn(["bun", "run", "src/index.ts", "run", "--request", "-", "--json", "--config", configPath], {
    cwd: process.cwd(),
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  proc.stdin.write(
    JSON.stringify({
      request: {
        adapter: "gemini",
        task: "Summarize the change",
        response_format: {
          type: "json_schema",
          schema: {
            type: "object",
            required: ["summary", "files"],
            properties: {
              summary: { type: "string" },
              files: { type: "array", items: { type: "string" } },
            },
            additionalProperties: false,
          },
        },
      },
      approval_granted: true,
    }),
  );
  proc.stdin.end();

  const payload = JSON.parse(await new Response(proc.stdout).text());
  expect(payload.status).toBe("completed");
  expect(payload.structured_output.summary).toBe("ok");
});

test("invalid structured output fails with report artifact", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
echo '{"summary":42}'
`,
    { mode: 0o755 },
  );

  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        gemini: {
          enabled: true,
          command: adapterPath,
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          supported_capabilities: ["read"],
        },
      },
    }),
  );

  const proc = Bun.spawn(["bun", "run", "src/index.ts", "run", "--request", "-", "--json", "--config", configPath], {
    cwd: process.cwd(),
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  proc.stdin.write(
    JSON.stringify({
      request: {
        adapter: "gemini",
        task: "Summarize the change",
        response_format: {
          type: "json_schema",
          schema: {
            type: "object",
            required: ["summary"],
            properties: { summary: { type: "string" } },
            additionalProperties: false,
          },
        },
      },
      approval_granted: true,
    }),
  );
  proc.stdin.end();

  const payload = JSON.parse(await new Response(proc.stdout).text());
  expect(payload.status).toBe("failed");
  expect(payload.artifacts.find((item: { kind: string }) => item.kind === "report")).toBeTruthy();
});

test("model aliases resolve to configured Gemini preview ids", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
echo "alias ok"
`,
    { mode: 0o755 },
  );

  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        gemini: {
          enabled: true,
          command: adapterPath,
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          default_model: "gemini-3.1-pro-preview",
          models: [{ id: "gemini-3.1-pro-preview", aliases: ["gemini-3.1-pro"] }],
        },
      },
    }),
  );

  const proc = Bun.spawn(
    ["bun", "run", "src/index.ts", "run", "-p", "Read-only alias test", "-m", "gemini/gemini-3.1-pro", "--config", configPath],
    { cwd: process.cwd(), stdout: "pipe", stderr: "pipe" },
  );

  const payload = JSON.parse(await new Response(proc.stdout).text());
  expect(payload.status).toBe("completed");
  expect(payload.adapter).toBe("gemini");
  expect(payload.final_text).toBe("alias ok");
});
