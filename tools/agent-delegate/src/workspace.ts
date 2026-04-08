import { mkdtemp, readFile, readdir, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";

import { AdapterConfig, Artifact, ChangeSet, NormalizedRequest, WorkspaceSnapshot, ignoredArtifactDirs, internalCodexOutput, maxArtifactBytes } from "./types";
import { cleanRelativePath } from "./request";

export function resolveBaseDir(rawPath: string) {
  if (!rawPath.trim()) {
    return invocationCwd();
  }
  return path.isAbsolute(rawPath) ? path.normalize(rawPath) : path.join(invocationCwd(), rawPath);
}

export function resolvePath(rawPath: string) {
  if (rawPath === "-") {
    return rawPath;
  }
  return path.isAbsolute(rawPath) ? rawPath : path.join(invocationCwd(), rawPath);
}

export async function prepareRunContext(baseDir: string, request: NormalizedRequest, allowedPaths: string[]) {
  const snapshot = await snapshotWorkspace(baseDir, allowedPaths);
  const inlineSections: string[] = [];
  const fileSections: string[] = [];

  for (const item of request.context) {
    switch (item.type) {
      case "inline":
        inlineSections.push(renderContextSection(item.label || "Inline context", String(item.text || "").trim()));
        break;
      case "search_results":
        inlineSections.push(renderContextSection(item.label || "Search results", String(item.text || "").trim()));
        break;
      case "command_output":
        inlineSections.push(renderCodeSection(item.label || "Command output", String(item.text || "").trim()));
        break;
      case "git_diff":
        inlineSections.push(renderCodeSection(item.label || "Git diff", String(item.text || "").trim()));
        break;
      case "glob": {
        const pattern = String(item.path || item.text || "").trim();
        if (!pattern) {
          throw new Error("glob context requires path or text");
        }
        const matches = await collectMatchingPaths(baseDir, allowedPaths, pattern);
        fileSections.push(renderContextSection(item.label || `Glob matches: ${pattern}`, matches.length > 0 ? matches.join("\n") : "(no matches)"));
        break;
      }
      case "snippet": {
        const { absolutePath, relativePath } = resolveContextPath(baseDir, item.path, allowedPaths);
        const content = await readFile(absolutePath, "utf8");
        fileSections.push(`### File snippet: ${relativePath}\n\n\`\`\`text\n${sliceLines(content, item.start_line, item.end_line)}\n\`\`\``);
        break;
      }
      case "file": {
        const { absolutePath, relativePath } = resolveContextPath(baseDir, item.path, allowedPaths);
        await stat(absolutePath);
        fileSections.push(`### File: ${relativePath}\n\nThis file is available in the working directory.`);
        break;
      }
      default:
        throw new Error(`unsupported context type ${JSON.stringify(item.type)}`);
    }
  }

  const promptBody = buildTaskDocument(baseDir, request, allowedPaths, inlineSections, fileSections);
  return { promptBody, snapshot };
}

export function buildArgs(request: NormalizedRequest, adapter: AdapterConfig, prompt: string, outputDir: string) {
  const args = [...(adapter.args ?? [])];
  switch (request.adapter) {
    case "gemini":
      if (request.model) {
        args.push("--model", request.model);
      }
      args.push("--prompt", prompt, "--output-format", "text", "--approval-mode", "yolo");
      return { args, outputFile: "" };
    case "claude":
      if (request.model) {
        args.push("--model", request.model);
      }
      args.push("-p", prompt, "--output-format", "text", "--permission-mode", "bypassPermissions");
      return { args, outputFile: "" };
    case "copilot":
      if (request.model) {
        args.push("--model", request.model);
      }
      args.push("-p", prompt, "-s", "--stream", "off", "--allow-all-tools");
      return { args, outputFile: "" };
    case "codex": {
      const outputFile = path.join(outputDir, internalCodexOutput);
      const sandbox = request.capabilities.includes("write") ? "workspace-write" : "read-only";
      args.push("exec");
      if (request.model) {
        args.push("-m", request.model);
      }
      args.push(prompt, "--skip-git-repo-check", "--color", "never", "-o", outputFile, "--sandbox", sandbox);
      if (request.mode === "guarded_execution" || request.capabilities.includes("exec")) {
        args.push("--full-auto");
      }
      return { args, outputFile };
    }
  }
}

export async function collectChangesAndArtifacts(rootDir: string, snapshot: WorkspaceSnapshot, allowedPaths: string[]) {
  const current = new Map<string, string | null>();
  await walkFiles(rootDir, rootDir, allowedPaths, async (absolutePath, relativePath) => {
    current.set(relativePath, await readTextArtifact(absolutePath));
  });

  const changes: ChangeSet = { created: [], updated: [], deleted: [] };
  const artifacts: Artifact[] = [];
  const allPaths = new Set<string>([...snapshot.keys(), ...current.keys()]);
  for (const relativePath of [...allPaths].sort((left, right) => left.localeCompare(right))) {
    const before = snapshot.get(relativePath);
    const after = current.get(relativePath);
    if (before === undefined && after !== undefined) {
      changes.created.push(relativePath);
      artifacts.push({ path: relativePath, kind: "created_file", content: after ?? undefined });
      continue;
    }
    if (before !== undefined && after === undefined) {
      changes.deleted.push(relativePath);
      artifacts.push({ path: relativePath, kind: "deleted_file" });
      continue;
    }
    if (before !== after) {
      changes.updated.push(relativePath);
      artifacts.push({ path: relativePath, kind: "updated_file", content: after ?? undefined });
    }
  }

  return { artifacts, changes };
}

export async function withOutputDir<T>(fn: (outputDir: string) => Promise<T>) {
  const outputDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-"));
  try {
    return await fn(outputDir);
  } finally {
    await rm(outputDir, { recursive: true, force: true });
  }
}

export async function readOptional(filePath: string) {
  if (!filePath) {
    return "";
  }
  try {
    return (await readFile(filePath, "utf8")).trim();
  } catch {
    return "";
  }
}

export function isPathAllowed(relativePath: string, allowedPaths: string[]) {
  if (allowedPaths.length === 0) {
    return true;
  }
  return allowedPaths.some((allowedPath) => allowedPath === "." || relativePath === allowedPath || relativePath.startsWith(`${allowedPath}/`));
}

export function resolveContextPath(baseDir: string, rawPath: string, allowedPaths: string[]) {
  const input = String(rawPath || "").trim();
  if (!input) {
    throw new Error("context file path is required");
  }
  const absolutePath = path.isAbsolute(input) ? path.normalize(input) : path.join(baseDir, input);
  const relativePath = path.relative(baseDir, absolutePath).replaceAll(path.sep, "/");
  if (relativePath === ".." || relativePath.startsWith("../")) {
    throw new Error(`context file ${JSON.stringify(rawPath)} escapes base directory`);
  }
  const cleaned = cleanRelativePath(relativePath || ".");
  if (!isPathAllowed(cleaned, allowedPaths)) {
    throw new Error(`context file ${JSON.stringify(rawPath)} is outside allowed_paths`);
  }
  return { absolutePath, relativePath: cleaned };
}

async function snapshotWorkspace(rootDir: string, allowedPaths: string[]): Promise<WorkspaceSnapshot> {
  const snapshot = new Map<string, string | null>();
  await walkFiles(rootDir, rootDir, allowedPaths, async (absolutePath, relativePath) => {
    snapshot.set(relativePath, await readTextArtifact(absolutePath));
  });
  return snapshot;
}

async function collectMatchingPaths(rootDir: string, allowedPaths: string[], pattern: string) {
  const matches: string[] = [];
  const matcher = globToRegExp(pattern);
  await walkFiles(rootDir, rootDir, allowedPaths, async (_absolutePath, relativePath) => {
    if (matcher.test(relativePath)) {
      matches.push(relativePath);
    }
  });
  return matches.sort((left, right) => left.localeCompare(right));
}

function globToRegExp(pattern: string) {
  let regex = "^";
  const normalized = String(pattern || "").trim().replaceAll("\\", "/");
  for (let index = 0; index < normalized.length; index += 1) {
    const char = normalized[index];
    const next = normalized[index + 1];
    if (char === "*" && next === "*") {
      regex += ".*";
      index += 1;
      continue;
    }
    if (char === "*") {
      regex += "[^/]*";
      continue;
    }
    if (char === "?") {
      regex += ".";
      continue;
    }
    regex += escapeRegExp(char);
  }
  regex += "$";
  return new RegExp(regex);
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function sliceLines(content: string, startLine?: number, endLine?: number) {
  const lines = content.split("\n");
  const start = startLine && startLine > 0 ? startLine : 1;
  const end = endLine && endLine > 0 ? Math.min(endLine, lines.length) : lines.length;
  if (start > end || start > lines.length) {
    throw new Error("invalid snippet line range");
  }
  return lines.slice(start - 1, end).join("\n");
}

function buildTaskDocument(baseDir: string, request: NormalizedRequest, allowedPaths: string[], inlineSections: string[], fileSections: string[]) {
  const parts = [
    "# Delegated Task",
    "",
    `Mode: ${request.mode}`,
    `Adapter: ${request.adapter}`,
    ...(request.model ? [`Model: ${request.model}`] : []),
    `Working directory: ${baseDir}`,
    `Capabilities: ${request.capabilities.join(", ")}`,
    `Allowed paths: ${allowedPaths.length > 0 ? allowedPaths.join(", ") : "(entire working directory)"}`,
    "",
    "## Instructions",
    "",
    "- Work only inside the specified working directory.",
    "- Obey the declared capability set. Do not write files unless write is allowed.",
    "- Do not access or modify files outside the allowed paths.",
    "- Treat the listed context items as the highest-priority starting point, but you may inspect other allowed files if the task requires it.",
    "- Keep the final answer concise and directly useful to the calling agent.",
  ];

  if (request.response_format.type === "json_schema") {
    parts.push(
      "",
      "## Output contract",
      "",
      "- Return valid JSON only.",
      "- Do not wrap the JSON in markdown fences.",
      "- The JSON must satisfy this schema:",
      "",
      "```json",
      JSON.stringify(request.response_format.schema, null, 2),
      "```",
    );
  }

  if (Object.keys(request.metadata).length > 0) {
    parts.push("", "## Metadata", "");
    for (const [key, value] of Object.entries(request.metadata)) {
      parts.push(`- ${key}: ${String(value)}`);
    }
  }
  if (inlineSections.length > 0) {
    parts.push("", "## Inline Context", "", inlineSections.join("\n\n"));
  }
  if (fileSections.length > 0) {
    parts.push("", "## File Context", "", fileSections.join("\n\n"));
  }
  parts.push("", "## Task", "", request.task, "");
  return parts.join("\n");
}

function renderContextSection(label: string, text: string) {
  return `### ${label}\n\n${text}`;
}

function renderCodeSection(label: string, text: string) {
  return `### ${label}\n\n\`\`\`text\n${text}\n\`\`\``;
}

async function walkFiles(baseDir: string, currentDir: string, allowedPaths: string[], visit: (absolutePath: string, relativePath: string) => Promise<void>) {
  const entries = await readdir(currentDir, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.isDirectory() && ignoredArtifactDirs.has(entry.name)) {
      continue;
    }
    const absolutePath = path.join(currentDir, entry.name);
    const relativePath = cleanRelativePath(path.relative(baseDir, absolutePath).replaceAll(path.sep, "/"));
    if (entry.isDirectory()) {
      if (allowedPaths.length > 0 && !directoryMayContainAllowed(relativePath, allowedPaths)) {
        continue;
      }
      await walkFiles(baseDir, absolutePath, allowedPaths, visit);
      continue;
    }
    if (!entry.isFile() || !isPathAllowed(relativePath, allowedPaths)) {
      continue;
    }
    await visit(absolutePath, relativePath);
  }
}

function directoryMayContainAllowed(relativePath: string, allowedPaths: string[]) {
  if (allowedPaths.length === 0 || relativePath === ".") {
    return true;
  }
  return allowedPaths.some((allowedPath) => allowedPath === relativePath || allowedPath.startsWith(`${relativePath}/`) || relativePath.startsWith(`${allowedPath}/`));
}

async function readTextArtifact(absolutePath: string) {
  const info = await stat(absolutePath);
  if (info.size > maxArtifactBytes) {
    return null;
  }
  try {
    return await readFile(absolutePath, "utf8");
  } catch {
    return null;
  }
}

function invocationCwd() {
  const callerCwd = String(process.env.AGENT_DELEGATE_CALLER_CWD || "").trim();
  return callerCwd || process.cwd();
}
