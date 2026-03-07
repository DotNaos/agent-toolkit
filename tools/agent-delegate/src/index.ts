import { mkdtemp, readFile, readdir, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";

type Mode = "advisory" | "guarded_execution";
type Status = "completed" | "failed" | "blocked" | "timed_out";
type AdapterId = "gemini" | "claude" | "copilot" | "codex";

type ContextItem = {
  type?: "inline" | "file" | "snippet";
  label?: string;
  text?: string;
  path?: string;
  start_line?: number;
  end_line?: number;
};

type Request = {
  adapter: AdapterId;
  model?: string;
  task: string;
  mode?: Mode;
  cwd?: string;
  context?: ContextItem[];
  timeout_sec?: number;
  metadata?: Record<string, unknown>;
};

type Risk = {
  approval_required: boolean;
  reason?: string;
};

type Artifact = {
  path: string;
  kind: "text_file";
  content?: string;
};

type Result = {
  status: Status;
  adapter: string;
  mode: Mode;
  final_text?: string;
  stdout?: string;
  stderr?: string;
  exit_code?: number;
  duration_ms: number;
  artifacts?: Artifact[];
  risk: Risk;
};

type DefaultsConfig = {
  timeout_sec?: number;
  max_timeout_sec?: number;
};

type AdapterConfig = {
  enabled: boolean;
  command: string;
  args?: string[];
  timeout_sec?: number;
  supports_guarded_execution: boolean;
  default_model?: string;
  models?: ModelConfig[];
};

type ModelConfig = {
  id: string;
  label?: string;
  multiplier?: number;
};

type Config = {
  defaults?: DefaultsConfig;
  adapters: Record<string, AdapterConfig>;
};

type BridgeRequest = {
  request: Request;
  approval_granted?: boolean;
};

const riskyKeywords = ["delete", "deploy", "publish", "run migration", "drop", "kill", "charge", "send email"];
const safeKeywords = ["read", "analyze", "inspect", "list", "summarize", "explain", "review", "show", "design"];
const internalCodexOutput = ".delegate-codex-last-message.txt";
const maxArtifactBytes = 64 * 1024;
const ignoredArtifactDirs = new Set([".git", "node_modules", ".next", "dist", "build", "coverage", ".turbo", ".cache"]);
const adapters: AdapterId[] = ["gemini", "claude", "copilot", "codex"];

async function main() {
  const argv = process.argv.slice(2);
  const compactJSON = wantsCompactJSON(argv);
  try {
    const [command, ...args] = argv;
    switch (command) {
      case "run":
        await runCommand(args, compactJSON);
        return;
      case "validate-config":
        await validateConfigCommand(args, compactJSON);
        return;
      case "list-adapters":
        await listAdaptersCommand(args, compactJSON);
        return;
      default:
        throw new Error(`unknown command ${JSON.stringify(command ?? "")}`);
    }
  } catch (error) {
    writeJSON({ status: "error", message: formatError(error) }, compactJSON);
    process.exitCode = 1;
  }
}

function parseFlags(args: string[]) {
  const aliases = new Map<string, string>([
    ["c", "config"],
    ["r", "request"],
    ["m", "model"],
    ["p", "prompt"],
    ["j", "json"],
  ]);
  const flags = new Map<string, string | boolean>();
  const positionals: string[] = [];
  for (let index = 0; index < args.length; index += 1) {
    const token = args[index];
    if (token === "--") {
      positionals.push(...args.slice(index + 1));
      break;
    }
    if (token.startsWith("--")) {
      const raw = token.slice(2);
      const equalsIndex = raw.indexOf("=");
      if (equalsIndex >= 0) {
        flags.set(raw.slice(0, equalsIndex), raw.slice(equalsIndex + 1));
        continue;
      }
      const next = args[index + 1];
      if (next && next !== "--" && (next === "-" || !next.startsWith("-"))) {
        flags.set(raw, next);
        index += 1;
        continue;
      }
      flags.set(raw, true);
      continue;
    }
    if (token.startsWith("-") && token.length === 2) {
      const key = aliases.get(token.slice(1));
      if (!key) {
        positionals.push(token);
        continue;
      }
      const next = args[index + 1];
      if (next && next !== "--" && (next === "-" || !next.startsWith("-"))) {
        flags.set(key, next);
        index += 1;
        continue;
      }
      flags.set(key, true);
      continue;
    }
    positionals.push(token);
  }
  return { flags, positionals };
}

function wantsCompactJSON(args: string[]) {
  return args.includes("--json") || args.includes("-j");
}

function writeJSON(value: unknown, compact: boolean) {
  console.log(JSON.stringify(value, null, compact ? undefined : 2));
}

function readStringFlag(parsed: ReturnType<typeof parseFlags>, key: string) {
  const value = parsed.flags.get(key);
  return typeof value === "string" ? value : "";
}

function hasFlag(parsed: ReturnType<typeof parseFlags>, key: string) {
  return parsed.flags.has(key);
}

async function runCommand(args: string[], compactJSON: boolean) {
  const parsed = parseFlags(args);
  const configPath = readStringFlag(parsed, "config") || "agent-delegate.json";
  const wrapped = await parseRunPayload(parsed);
  const result = await runDelegate(wrapped.request, Boolean(wrapped.approval_granted), configPath);
  writeJSON(result, compactJSON);
}

async function validateConfigCommand(args: string[], compactJSON: boolean) {
  const parsed = parseFlags(args);
  const configPath = readStringFlag(parsed, "config") || "agent-delegate.json";
  const config = await loadConfig(configPath);
  writeJSON({ status: "success", action: "validate-config", config: configPath, adapters: listEnabledAdapters(config) }, compactJSON);
}

async function listAdaptersCommand(args: string[], compactJSON: boolean) {
  const parsed = parseFlags(args);
  const configPath = readStringFlag(parsed, "config") || "agent-delegate.json";
  const config = await loadConfig(configPath);
  writeJSON({ status: "success", action: "list-adapters", adapters: listEnabledAdapters(config) }, compactJSON);
}

function isBridgeRequest(value: BridgeRequest | Request): value is BridgeRequest {
  return Object.prototype.hasOwnProperty.call(value, "request");
}

async function readJSONInput<T>(requestPath: string): Promise<T> {
  const raw = requestPath === "-" ? await new Response(Bun.stdin.stream()).text() : await readFile(resolvePath(requestPath), "utf8");
  return JSON.parse(raw) as T;
}

async function parseRunPayload(parsed: ReturnType<typeof parseFlags>): Promise<BridgeRequest> {
  const requestPath = readStringFlag(parsed, "request");
  const prompt = readStringFlag(parsed, "prompt").trim();
  if (!requestPath && !prompt) {
    throw new Error("run requires --request or --prompt");
  }

  let wrapped: BridgeRequest;
  if (requestPath) {
    const bridgePayload = (await readJSONInput<BridgeRequest | Request>(requestPath)) as BridgeRequest | Request;
    wrapped = isBridgeRequest(bridgePayload)
      ? bridgePayload
      : { request: bridgePayload, approval_granted: false };
  } else {
    wrapped = {
      request: { adapter: "" as AdapterId, task: prompt },
      approval_granted: false,
    };
  }

  const request = applyRunFlagOverrides(wrapped.request, parsed);
  return {
    request,
    approval_granted: hasFlag(parsed, "approval-granted") ? true : Boolean(wrapped.approval_granted),
  };
}

function applyRunFlagOverrides(input: Request, parsed: ReturnType<typeof parseFlags>): Request {
  const request: Request = {
    adapter: String(input.adapter || "").trim().toLowerCase() as AdapterId,
    model: String(input.model || "").trim(),
    task: String(input.task || "").trim(),
    mode: input.mode,
    cwd: input.cwd ? String(input.cwd) : "",
    context: Array.isArray(input.context) ? input.context : [],
    timeout_sec: typeof input.timeout_sec === "number" ? input.timeout_sec : undefined,
    metadata: input.metadata ?? {},
  };

  const prompt = readStringFlag(parsed, "prompt").trim();
  if (prompt) {
    request.task = prompt;
  }

  const mode = readStringFlag(parsed, "mode").trim();
  if (mode) {
    request.mode = mode as Mode;
  }

  const cwd = readStringFlag(parsed, "cwd").trim();
  if (cwd) {
    request.cwd = cwd;
  }

  const timeoutRaw = readStringFlag(parsed, "timeout-sec").trim() || readStringFlag(parsed, "timeout").trim();
  if (timeoutRaw) {
    const timeoutSec = Number.parseInt(timeoutRaw, 10);
    if (!Number.isFinite(timeoutSec) || timeoutSec < 0) {
      throw new Error(`invalid timeout ${JSON.stringify(timeoutRaw)}`);
    }
    request.timeout_sec = timeoutSec;
  }

  const adapterOverride = readStringFlag(parsed, "adapter").trim().toLowerCase();
  if (adapterOverride) {
    if (!adapters.includes(adapterOverride as AdapterId)) {
      throw new Error(`unsupported adapter ${JSON.stringify(adapterOverride)}`);
    }
    request.adapter = adapterOverride as AdapterId;
  }

  const requestModel = parseQualifiedModel(request.model);
  if (requestModel.adapter) {
    if (request.adapter && request.adapter !== requestModel.adapter) {
      throw new Error(`request model selector adapter ${JSON.stringify(requestModel.adapter)} conflicts with adapter ${JSON.stringify(request.adapter)}`);
    }
    request.adapter = requestModel.adapter;
  }
  request.model = normalizeModelSelection(request.adapter, request.model, "request");

  const modelOverride = readStringFlag(parsed, "model").trim();
  if (modelOverride) {
    request.model = normalizeModelSelection(request.adapter, modelOverride, "flag");
    const modelAdapter = parseQualifiedModel(modelOverride).adapter;
    if (modelAdapter) {
      request.adapter = modelAdapter;
    }
  }

  return request;
}

function parseQualifiedModel(rawValue: string) {
  const trimmed = String(rawValue || "").trim();
  if (!trimmed) {
    return { adapter: "" as AdapterId | "", model: "" };
  }

  const slashIndex = trimmed.indexOf("/");
  if (slashIndex < 0) {
    return { adapter: "" as AdapterId | "", model: trimmed };
  }

  const adapter = trimmed.slice(0, slashIndex).trim().toLowerCase();
  const model = trimmed.slice(slashIndex + 1).trim();
  if (!adapter || !model) {
    throw new Error(`invalid model selector ${JSON.stringify(trimmed)}`);
  }
  if (!adapters.includes(adapter as AdapterId)) {
    throw new Error(`unsupported adapter ${JSON.stringify(adapter)} in model selector`);
  }
  return { adapter: adapter as AdapterId, model };
}

function normalizeModelSelection(currentAdapter: string, rawValue: string, source: "request" | "flag") {
  const parsed = parseQualifiedModel(rawValue);
  if (!parsed.model) {
    return "";
  }
  if (parsed.adapter && currentAdapter && parsed.adapter !== currentAdapter) {
    throw new Error(`${source} model selector adapter ${JSON.stringify(parsed.adapter)} conflicts with adapter ${JSON.stringify(currentAdapter)}`);
  }
  if (!parsed.adapter && source === "flag" && !currentAdapter) {
    throw new Error("flag model selector must use adapter/model when no adapter is set");
  }
  return parsed.model;
}

async function loadConfig(configPath: string): Promise<Required<Config>> {
  const data = await readFile(resolvePath(configPath), "utf8");
  const config = JSON.parse(data) as Config;
  const normalized: Required<Config> = {
    defaults: {
      timeout_sec: config.defaults?.timeout_sec && config.defaults.timeout_sec > 0 ? config.defaults.timeout_sec : 120,
      max_timeout_sec: config.defaults?.max_timeout_sec && config.defaults.max_timeout_sec > 0 ? config.defaults.max_timeout_sec : 600,
    },
    adapters: config.adapters ?? {},
  };

  if (Object.keys(normalized.adapters).length === 0) {
    throw new Error("config must define at least one adapter");
  }
  for (const [id, adapter] of Object.entries(normalized.adapters)) {
    if (!adapter.enabled) {
      continue;
    }
    if (!adapter.command?.trim()) {
      throw new Error(`adapter ${JSON.stringify(id)} missing command`);
    }
  }
  return normalized;
}

function listEnabledAdapters(config: Required<Config>) {
  return Object.entries(config.adapters)
    .filter(([, adapter]) => adapter.enabled)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([id, adapter]) => ({
      id,
      command: resolveAdapterCommand(id, adapter),
      args: adapter.args ?? [],
      timeout_sec: adapter.timeout_sec ?? config.defaults.timeout_sec,
      supports_guarded_execution: adapter.supports_guarded_execution,
      default_model: adapter.default_model ?? "",
      models: (adapter.models ?? []).map((model) => ({
        ...model,
        qualified_id: `${id}/${model.id}`,
      })),
    }));
}

function resolveAdapterCommand(adapterId: string, adapter: AdapterConfig) {
  const envName = `AGENT_DELEGATE_${adapterId.toUpperCase().replace(/-/g, "_")}_COMMAND`;
  return String(process.env[envName] || adapter.command);
}

async function runDelegate(rawRequest: Request, approvalGranted: boolean, configPath: string): Promise<Result> {
  const request = normalizeRequest(rawRequest);
  validateRequest(request);

  const config = await loadConfig(configPath);
  const adapter = config.adapters[request.adapter];
  if (!adapter || !adapter.enabled) {
    return blockedResult(request, { approval_required: false, reason: "adapter is not enabled" });
  }
  const resolvedModel = resolveRequestModel(request, adapter);

  const risk = assessRisk(request.task, request.metadata ?? {}, request.mode);
  if (risk.approval_required && !approvalGranted) {
    return blockedResult(request, risk);
  }
  if (request.mode === "guarded_execution" && !adapter.supports_guarded_execution) {
    return blockedResult(request, { approval_required: false, reason: "adapter does not support guarded_execution" });
  }

  const timeoutSec = clampTimeout(request.timeout_sec, adapter.timeout_sec, config.defaults.timeout_sec, config.defaults.max_timeout_sec);
  const baseDir = resolveBaseDir(request.cwd);
  const outputDir = await mkdtemp(path.join(os.tmpdir(), "agent-delegate-"));
  const startedAt = Date.now();

  try {
    const { promptBody, snapshot } = await prepareRunContext(baseDir, request);

    const command = resolveAdapterCommand(request.adapter, adapter);
    const { args, outputFile } = buildArgs({ ...request, model: resolvedModel }, adapter, promptBody, outputDir);
    const proc = Bun.spawn([command, ...args], {
      cwd: baseDir,
      env: process.env,
      stdout: "pipe",
      stderr: "pipe",
    });

    const timer = setTimeout(() => proc.kill(), timeoutSec * 1000);
    const [stdout, stderr, exitCode] = await Promise.all([
      new Response(proc.stdout).text(),
      new Response(proc.stderr).text(),
      proc.exited,
    ]).finally(() => clearTimeout(timer));

    const duration = Date.now() - startedAt;
    const finalText = outputFile ? await readOptional(outputFile) : stdout.trim();
    const artifacts = await collectArtifacts(baseDir, snapshot);

    if (proc.signalCode !== null) {
      return {
        status: "timed_out",
        adapter: request.adapter,
        mode: request.mode,
        stdout,
        stderr,
        exit_code: exitCode,
        duration_ms: duration,
        risk,
      };
    }

    return {
      status: exitCode === 0 ? "completed" : "failed",
      adapter: request.adapter,
      mode: request.mode,
      final_text: finalText || stdout.trim(),
      stdout,
      stderr,
      exit_code: exitCode,
      duration_ms: duration,
      artifacts,
      risk,
    };
  } finally {
    await rm(outputDir, { recursive: true, force: true });
  }
}

function normalizeRequest(request: Request): Required<Request> {
  return {
    adapter: String(request.adapter || "").trim().toLowerCase() as AdapterId,
    model: String(request.model || "").trim(),
    task: String(request.task || "").trim(),
    mode: (request.mode || "advisory") as Mode,
    cwd: request.cwd ? String(request.cwd) : "",
    context: Array.isArray(request.context) ? request.context : [],
    timeout_sec: typeof request.timeout_sec === "number" ? request.timeout_sec : 0,
    metadata: request.metadata ?? {},
  };
}

function validateRequest(request: Required<Request>) {
  if (!adapters.includes(request.adapter)) {
    throw new Error(`unsupported adapter ${JSON.stringify(request.adapter)}`);
  }
  if (!request.task) {
    throw new Error("task is required");
  }
  if (!["advisory", "guarded_execution"].includes(request.mode)) {
    throw new Error(`invalid mode ${JSON.stringify(request.mode)}`);
  }
}

function resolveRequestModel(request: Required<Request>, adapter: AdapterConfig) {
  const requested = request.model.trim();
  const available = (adapter.models ?? []).map((item) => item.id);
  if (!requested) {
    return adapter.default_model?.trim() ?? "";
  }
  if (available.length > 0 && !available.includes(requested)) {
    throw new Error(`model ${JSON.stringify(requested)} is not configured for adapter ${JSON.stringify(request.adapter)}`);
  }
  return requested;
}

function assessRisk(task: string, metadata: Record<string, unknown>, mode: Mode): Risk {
  if (mode === "guarded_execution") {
    return { approval_required: true, reason: "guarded_execution requires human approval" };
  }

  const action = String(metadata.action ?? "").toLowerCase().trim();
  if (action.includes("deploy") || action.includes("delete") || action.includes("publish") || action.includes("migration")) {
    return { approval_required: true, reason: "metadata action indicates side effects" };
  }
  if (action.includes("read") || action.includes("analyze")) {
    return { approval_required: false, reason: "metadata action indicates read-only" };
  }

  const lowerTask = task.toLowerCase();
  for (const keyword of riskyKeywords) {
    if (lowerTask.includes(keyword)) {
      return { approval_required: true, reason: `task contains risky keyword: ${keyword}` };
    }
  }
  for (const keyword of safeKeywords) {
    if (lowerTask.includes(keyword)) {
      return { approval_required: false, reason: "task appears read-only" };
    }
  }
  return { approval_required: false, reason: "advisory mode defaults to no approval" };
}

function blockedResult(request: Required<Request>, risk: Risk): Result {
  return {
    status: "blocked",
    adapter: request.adapter,
    mode: request.mode,
    duration_ms: 0,
    risk,
  };
}

function clampTimeout(requested: number, adapterDefault: number | undefined, repoDefault: number, maxTimeout: number) {
  const resolved = requested > 0 ? requested : adapterDefault && adapterDefault > 0 ? adapterDefault : repoDefault;
  return resolved > maxTimeout ? maxTimeout : resolved;
}

function resolveBaseDir(rawPath: string) {
  if (!rawPath.trim()) {
    return invocationCwd();
  }
  return path.isAbsolute(rawPath) ? path.normalize(rawPath) : path.join(invocationCwd(), rawPath);
}

async function prepareRunContext(baseDir: string, request: Required<Request>) {
  const snapshot = await snapshotWorkspace(baseDir);
  const inlineSections: string[] = [];
  const fileSections: string[] = [];

  for (const item of request.context) {
    const normalized = normalizeContextItem(item);
    if (normalized.type === "inline") {
      inlineSections.push(`### ${normalized.label?.trim() || "Inline context"}\n\n${String(normalized.text || "").trim()}`);
      continue;
    }

    const { absolutePath, relativePath } = resolveContextPath(baseDir, String(normalized.path || ""));
    if (normalized.type === "snippet") {
      const content = await readFile(absolutePath, "utf8");
      fileSections.push(`### File snippet: ${relativePath}\n\n\`\`\`text\n${sliceLines(content, normalized.start_line, normalized.end_line)}\n\`\`\``);
      continue;
    }

    await stat(absolutePath);
    fileSections.push(`### File: ${relativePath}\n\nThis file is available in the working directory.`);
  }

  const promptBody = buildTaskDocument(baseDir, request, inlineSections, fileSections);
  return { promptBody, snapshot };
}

function normalizeContextItem(item: ContextItem): Required<ContextItem> {
  const type = item.type || (item.path ? (item.start_line || item.end_line ? "snippet" : "file") : "inline");
  return {
    type,
    label: item.label || "",
    text: item.text || "",
    path: item.path || "",
    start_line: item.start_line || 0,
    end_line: item.end_line || 0,
  };
}

function resolveContextPath(baseDir: string, rawPath: string) {
  if (!rawPath.trim()) {
    throw new Error("context file path is required");
  }
  const absolutePath = path.isAbsolute(rawPath) ? path.normalize(rawPath) : path.join(baseDir, rawPath);
  const relativePath = path.relative(baseDir, absolutePath);
  if (relativePath === ".." || relativePath.startsWith(`..${path.sep}`)) {
    throw new Error(`context file ${JSON.stringify(rawPath)} escapes base directory`);
  }
  return { absolutePath, relativePath };
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

function buildTaskDocument(baseDir: string, request: Required<Request>, inlineSections: string[], fileSections: string[]) {
  const parts = [
    "# Delegated Task",
    "",
    `Mode: ${request.mode}`,
    `Adapter: ${request.adapter}`,
    ...(request.model ? [`Model: ${request.model}`] : []),
    `Working directory: ${baseDir}`,
    "",
    "## Instructions",
    "",
    "- Work only inside the specified working directory.",
    "- You may read and write files in this working directory when needed for the task.",
    "- Do not access or modify files outside this working directory.",
    "- Treat the listed context files as the highest-priority starting point, but you may inspect other files under the working directory if the task requires it.",
    "- Keep the final answer concise and directly useful to the calling agent.",
  ];

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

function buildArgs(request: Required<Request>, adapter: AdapterConfig, prompt: string, outputDir: string) {
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
      args.push("exec");
      if (request.model) {
        args.push("-m", request.model);
      }
      args.push(prompt, "--skip-git-repo-check", "--color", "never", "-o", outputFile, "--sandbox", "workspace-write");
      if (request.mode === "guarded_execution") {
        args.push("--full-auto");
      }
      return { args, outputFile };
    }
  }
}

async function readOptional(filePath: string) {
  if (!filePath) {
    return "";
  }
  try {
    return (await readFile(filePath, "utf8")).trim();
  } catch {
    return "";
  }
}

async function snapshotWorkspace(rootDir: string): Promise<Map<string, string>> {
  const snapshot = new Map<string, string>();
  await walkFiles(rootDir, async (absolutePath) => {
    const relativePath = path.relative(rootDir, absolutePath).replaceAll(path.sep, "/");
    const content = await readTextArtifact(absolutePath);
    if (content === null) {
      return;
    }
    snapshot.set(relativePath, content);
  });
  return snapshot;
}

async function collectArtifacts(rootDir: string, snapshot: Map<string, string>): Promise<Artifact[]> {
  const artifacts: Artifact[] = [];
  await walkFiles(rootDir, async (absolutePath) => {
    const relativePath = path.relative(rootDir, absolutePath).replaceAll(path.sep, "/");
    const content = await readTextArtifact(absolutePath);
    if (content === null) {
      return;
    }
    if (snapshot.get(relativePath) === content) {
      return;
    }
    artifacts.push({ path: relativePath, kind: "text_file", content });
  });
  return artifacts;
}

async function walkFiles(rootDir: string, visit: (absolutePath: string) => Promise<void>) {
  const entries = await readdir(rootDir, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.isDirectory() && ignoredArtifactDirs.has(entry.name)) {
      continue;
    }
    const absolutePath = path.join(rootDir, entry.name);
    if (entry.isDirectory()) {
      await walkFiles(absolutePath, visit);
      continue;
    }
    if (!entry.isFile()) {
      continue;
    }
    await visit(absolutePath);
  }
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

function resolvePath(rawPath: string) {
  if (rawPath === "-") {
    return rawPath;
  }
  return path.isAbsolute(rawPath) ? rawPath : path.join(invocationCwd(), rawPath);
}

function formatError(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function invocationCwd() {
  const callerCwd = String(process.env.AGENT_DELEGATE_CALLER_CWD || "").trim();
  return callerCwd || process.cwd();
}

void main();
