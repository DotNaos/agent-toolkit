import { loadConfig, listEnabledAdapters, resolveAdapterCommand } from "./config";
import { evaluatePolicy } from "./policy";
import { finalizeOutput } from "./schema";
import { buildPolicyDecision, normalizeModelSelection, normalizeRequest, normalizeAllowedPaths, parseQualifiedModel, policyToRisk, validateRequest } from "./request";
import { AdapterId, BridgeRequest, ModelConfig, Request } from "./types";
import { buildArgs, collectChangesAndArtifacts, prepareRunContext, readOptional, resolveBaseDir, resolvePath, withOutputDir } from "./workspace";

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
  const raw = requestPath === "-" ? await new Response(Bun.stdin.stream()).text() : await Bun.file(resolvePath(requestPath)).text();
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
    const bridgePayload = await readJSONInput<BridgeRequest | Request>(requestPath);
    wrapped = isBridgeRequest(bridgePayload) ? bridgePayload : { request: bridgePayload, approval_granted: false };
  } else {
    wrapped = { request: { adapter: "" as AdapterId, task: prompt }, approval_granted: false };
  }

  return {
    request: applyRunFlagOverrides(wrapped.request, parsed),
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
    capabilities: Array.isArray(input.capabilities) ? input.capabilities : [],
    allowed_paths: Array.isArray(input.allowed_paths) ? input.allowed_paths : [],
    response_format: input.response_format,
  };

  const prompt = readStringFlag(parsed, "prompt").trim();
  if (prompt) {
    request.task = prompt;
  }

  const mode = readStringFlag(parsed, "mode").trim();
  if (mode) {
    request.mode = mode as Request["mode"];
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

async function runDelegate(rawRequest: Request, approvalGranted: boolean, configPath: string) {
  const config = await loadConfig(configPath);
  const request = normalizeRequest(rawRequest, config.policy);
  validateRequest(request);

  const adapter = config.adapters[request.adapter];
  if (!adapter || !adapter.enabled) {
    const policy = buildPolicyDecision(request.capabilities, [], false, "adapter is not enabled");
    return { status: "blocked" as const, adapter: request.adapter, mode: request.mode, duration_ms: 0, policy, risk: policyToRisk(policy) };
  }

  const resolvedModel = resolveConfiguredModel(request.model, adapter.default_model, adapter.models ?? [], request.adapter);

  const baseDir = resolveBaseDir(request.cwd);
  const allowedPaths = normalizeAllowedPaths(request.allowed_paths, baseDir);
  const policy = evaluatePolicy(request, config.policy, adapter, approvalGranted);
  if (policy.blocked_reason) {
    return { status: "blocked" as const, adapter: request.adapter, mode: request.mode, duration_ms: 0, policy: policy.decision, risk: policyToRisk(policy.decision) };
  }

  const timeoutSec = Math.min(request.timeout_sec > 0 ? request.timeout_sec : adapter.timeout_sec || config.defaults.timeout_sec, config.defaults.max_timeout_sec);
  return withOutputDir(async (outputDir) => {
    const startedAt = Date.now();
    const { promptBody, snapshot } = await prepareRunContext(baseDir, { ...request, model: resolvedModel }, allowedPaths);
    const { args, outputFile } = buildArgs({ ...request, model: resolvedModel }, adapter, promptBody, outputDir);
    const proc = Bun.spawn([resolveAdapterCommand(request.adapter, adapter), ...args], {
      cwd: baseDir,
      env: process.env,
      stdout: "pipe",
      stderr: "pipe",
    });

    const timer = setTimeout(() => proc.kill(), timeoutSec * 1000);
    const [stdout, stderr, exitCode] = await Promise.all([new Response(proc.stdout).text(), new Response(proc.stderr).text(), proc.exited]).finally(() => clearTimeout(timer));
    const duration = Date.now() - startedAt;
    const finalText = outputFile ? await readOptional(outputFile) : stdout.trim();
    const { artifacts, changes } = await collectChangesAndArtifacts(baseDir, snapshot, allowedPaths);

    if (proc.signalCode !== null) {
      return {
        status: "timed_out" as const,
        adapter: request.adapter,
        mode: request.mode,
        stdout,
        stderr,
        exit_code: exitCode,
        duration_ms: duration,
        artifacts,
        changes,
        policy: policy.decision,
        risk: policyToRisk(policy.decision),
      };
    }

    const output = await finalizeOutput({ ...request, model: resolvedModel }, finalText || stdout.trim(), stdout, artifacts);
    return {
      status: output.status ?? (exitCode === 0 ? "completed" : "failed"),
      adapter: request.adapter,
      mode: request.mode,
      final_text: finalText || stdout.trim(),
      stdout,
      stderr,
      exit_code: exitCode,
      duration_ms: duration,
      artifacts: output.artifacts,
      structured_output: output.structured_output,
      changes,
      policy: policy.decision,
      risk: policyToRisk(policy.decision),
    };
  });
}

function resolveConfiguredModel(requestedModel: string, defaultModel: string | undefined, models: ModelConfig[], adapter: string) {
  const requested = requestedModel.trim();
  if (!requested) {
    return defaultModel?.trim() ?? "";
  }
  if (models.length === 0) {
    return requested;
  }
  const matched = models.find((model) => model.id === requested || (model.aliases ?? []).includes(requested) || model.label === requested);
  if (!matched) {
    throw new Error(`model ${JSON.stringify(requested)} is not configured for adapter ${JSON.stringify(adapter)}`);
  }
  return matched.id;
}

function formatError(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

void main();
