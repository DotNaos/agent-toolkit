import { test, expect } from "bun:test";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

test("run blocks guarded execution without approval", async () => {
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
      request: { adapter: "gemini", task: "edit the file", mode: "guarded_execution" },
      approval_granted: false,
    }),
  );
  proc.stdin.end();

  const output = await new Response(proc.stdout).text();
  const payload = JSON.parse(output);
  expect(payload.status).toBe("blocked");
});

test("run edits files directly in cwd and returns artifacts", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const projectDir = path.join(tempDir, "project");
  await mkdir(projectDir, { recursive: true });
  await writeFile(path.join(projectDir, "note.txt"), "before\n");

  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
echo "done"
printf "after\\n" > note.txt
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
        task: "review the context",
        mode: "advisory",
        cwd: projectDir,
        context: [{ type: "file", path: "note.txt" }],
        metadata: { action: "read" },
      },
      approval_granted: true,
    }),
  );
  proc.stdin.end();

  const output = await new Response(proc.stdout).text();
  const payload = JSON.parse(output);
  expect(payload.status).toBe("completed");
  expect(payload.artifacts[0].path).toBe("note.txt");
  expect(await readFile(path.join(projectDir, "note.txt"), "utf8")).toBe("after\n");
});

test("list-adapters returns configured models", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const configPath = path.join(tempDir, "agent-delegate.json");
  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        copilot: {
          enabled: true,
          command: "copilot",
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          default_model: "gpt-5.4",
          models: [
            { id: "gpt-5.4", multiplier: 1 },
            { id: "claude-sonnet-4.6", multiplier: 1 },
          ],
        },
      },
    }),
  );

  const proc = Bun.spawn(["bun", "run", "src/index.ts", "list-adapters", "--config", configPath], {
    cwd: process.cwd(),
    stdout: "pipe",
    stderr: "pipe",
  });

  const output = await new Response(proc.stdout).text();
  const payload = JSON.parse(output);
  expect(payload.adapters[0].default_model).toBe("gpt-5.4");
  expect(payload.adapters[0].models[1].multiplier).toBe(1);
  expect(payload.adapters[0].models[0].qualified_id).toBe("copilot/gpt-5.4");
});

test("run accepts prompt and qualified model flags", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
echo "delegate output"
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
          default_model: "gemini-2.5-pro",
          models: [{ id: "gemini-2.5-pro" }],
        },
      },
    }),
  );

  const proc = Bun.spawn(
    ["bun", "run", "src/index.ts", "run", "-p", "review the prompt", "-m", "gemini/gemini-2.5-pro", "--config", configPath],
    {
      cwd: process.cwd(),
      stdout: "pipe",
      stderr: "pipe",
    },
  );

  const output = await new Response(proc.stdout).text();
  const payload = JSON.parse(output);
  expect(payload.status).toBe("completed");
  expect(payload.adapter).toBe("gemini");
  expect(payload.final_text).toBe("delegate output");
  expect(output).toContain('\n  "status": "completed"');
});

test("advisory prompt without risk keywords does not require approval", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
echo "ok"
`,
    { mode: 0o755 },
  );

  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        copilot: {
          enabled: true,
          command: adapterPath,
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          default_model: "gpt-5-mini",
          models: [{ id: "gpt-5-mini" }],
        },
      },
    }),
  );

  const proc = Bun.spawn(
    ["bun", "run", "src/index.ts", "run", "-p", "Hello?", "-m", "copilot/gpt-5-mini", "--config", configPath],
    {
      cwd: process.cwd(),
      stdout: "pipe",
      stderr: "pipe",
    },
  );

  const output = await new Response(proc.stdout).text();
  const payload = JSON.parse(output);
  expect(payload.status).toBe("completed");
  expect(payload.risk.approval_required).toBe(false);
});

test("metadata action write no longer forces approval for cwd-local edits", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const projectDir = path.join(tempDir, "project");
  const adapterPath = path.join(tempDir, "fake-adapter.sh");
  const configPath = path.join(tempDir, "agent-delegate.json");

  await mkdir(projectDir, { recursive: true });
  await writeFile(path.join(projectDir, "note.txt"), "before\n");
  await writeFile(
    adapterPath,
    `#!/bin/sh
set -eu
printf "after\\n" > note.txt
echo "ok"
`,
    { mode: 0o755 },
  );
  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        codex: {
          enabled: true,
          command: adapterPath,
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          default_model: "gpt-5.4",
          models: [{ id: "gpt-5.4" }],
        },
      },
    }),
  );

  const proc = Bun.spawn(
    ["bun", "run", "src/index.ts", "run", "-p", "Update note.txt", "-m", "codex/gpt-5.4", "--config", configPath, "--cwd", projectDir],
    {
      cwd: process.cwd(),
      stdout: "pipe",
      stderr: "pipe",
      env: { ...process.env, AGENT_DELEGATE_CODEX_COMMAND: adapterPath },
    },
  );

  const output = await new Response(proc.stdout).text();
  const payload = JSON.parse(output);
  expect(payload.status).toBe("completed");
  expect(payload.risk.approval_required).toBe(false);
  expect(await readFile(path.join(projectDir, "note.txt"), "utf8")).toBe("after\n");
});

test("json flag emits compact output", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-test-"));
  const configPath = path.join(tempDir, "agent-delegate.json");

  await writeFile(
    configPath,
    JSON.stringify({
      defaults: { timeout_sec: 5, max_timeout_sec: 5 },
      adapters: {
        codex: {
          enabled: true,
          command: "codex",
          args: [],
          timeout_sec: 5,
          supports_guarded_execution: true,
          default_model: "gpt-5.4",
          models: [{ id: "gpt-5.4" }],
        },
      },
    }),
  );

  const prettyProc = Bun.spawn(["bun", "run", "src/index.ts", "list-adapters", "--config", configPath], {
    cwd: process.cwd(),
    stdout: "pipe",
    stderr: "pipe",
  });
  const compactProc = Bun.spawn(["bun", "run", "src/index.ts", "list-adapters", "--json", "--config", configPath], {
    cwd: process.cwd(),
    stdout: "pipe",
    stderr: "pipe",
  });

  const prettyOutput = await new Response(prettyProc.stdout).text();
  const compactOutput = await new Response(compactProc.stdout).text();

  expect(prettyOutput).toContain('\n  "status": "success"');
  expect(compactOutput.trim()).not.toContain("\n");
});
