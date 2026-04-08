import path from "node:path";

import {
  AdapterId,
  Capability,
  ContextItem,
  NormalizedRequest,
  PolicyDecision,
  PolicyConfig,
  Request,
  ResponseFormat,
  Risk,
  adapters,
  capabilities,
  guardedExecutionCapabilities,
} from "./types";

export function parseQualifiedModel(rawValue: string) {
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

export function normalizeModelSelection(currentAdapter: string, rawValue: string, source: "request" | "flag") {
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

export function normalizeCapabilities(value: unknown, label: string, fallback: Capability[] = []): Capability[] {
  if (!Array.isArray(value)) {
    return [...fallback];
  }
  const normalized: Capability[] = [];
  for (const item of value) {
    const capability = String(item || "").trim().toLowerCase() as Capability;
    if (!capability) {
      continue;
    }
    if (!capabilities.includes(capability)) {
      throw new Error(`unsupported capability ${JSON.stringify(capability)} in ${label}`);
    }
    if (!normalized.includes(capability)) {
      normalized.push(capability);
    }
  }
  return normalized.length > 0 ? normalized : [...fallback];
}

export function normalizeRequest(request: Request, policy: Required<PolicyConfig>): NormalizedRequest {
  const mode = (request.mode || "advisory") as NormalizedRequest["mode"];
  const explicitCapabilities = normalizeCapabilities(request.capabilities, "request capabilities", []);
  const derivedCapabilities = explicitCapabilities.length > 0 ? explicitCapabilities : [...policy.default_capabilities];
  if (mode === "guarded_execution") {
    for (const capability of guardedExecutionCapabilities) {
      if (!derivedCapabilities.includes(capability)) {
        derivedCapabilities.push(capability);
      }
    }
  }

  return {
    adapter: String(request.adapter || "").trim().toLowerCase() as AdapterId,
    model: String(request.model || "").trim(),
    task: String(request.task || "").trim(),
    mode,
    cwd: request.cwd ? String(request.cwd) : "",
    context: Array.isArray(request.context) ? request.context.map(normalizeContextItem) : [],
    timeout_sec: typeof request.timeout_sec === "number" ? request.timeout_sec : 0,
    metadata: request.metadata ?? {},
    capabilities: derivedCapabilities,
    allowed_paths: Array.isArray(request.allowed_paths) ? request.allowed_paths.map(String) : [],
    response_format: normalizeResponseFormat(request.response_format),
  };
}

export function validateRequest(request: NormalizedRequest) {
  if (!adapters.includes(request.adapter)) {
    throw new Error(`unsupported adapter ${JSON.stringify(request.adapter)}`);
  }
  if (!request.task) {
    throw new Error("task is required");
  }
  if (!["advisory", "guarded_execution"].includes(request.mode)) {
    throw new Error(`invalid mode ${JSON.stringify(request.mode)}`);
  }
  if (request.capabilities.length === 0) {
    throw new Error("request must resolve to at least one capability");
  }
}

export function normalizeContextItem(item: ContextItem): Required<ContextItem> {
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

export function normalizeResponseFormat(input?: ResponseFormat): Required<ResponseFormat> {
  const type = input?.type || "text";
  if (type !== "text" && type !== "json_schema") {
    throw new Error(`unsupported response_format type ${JSON.stringify(type)}`);
  }
  if (type === "json_schema" && !isPlainObject(input?.schema)) {
    throw new Error("response_format json_schema requires schema");
  }
  return {
    type,
    schema: type === "json_schema" ? input?.schema ?? {} : {},
  };
}

export function normalizeAllowedPaths(rawPaths: string[], baseDir: string) {
  if (!rawPaths || rawPaths.length === 0) {
    return [];
  }
  const normalized: string[] = [];
  for (const rawPath of rawPaths) {
    const value = String(rawPath || "").trim();
    if (!value) {
      continue;
    }
    const absolutePath = path.isAbsolute(value) ? path.normalize(value) : path.join(baseDir, value);
    const relativePath = path.relative(baseDir, absolutePath).replaceAll(path.sep, "/");
    if (relativePath === ".." || relativePath.startsWith("../")) {
      throw new Error(`allowed path ${JSON.stringify(value)} escapes base directory`);
    }
    const cleaned = cleanRelativePath(relativePath || ".");
    if (!normalized.includes(cleaned)) {
      normalized.push(cleaned);
    }
  }
  return normalized;
}

export function cleanRelativePath(rawPath: string) {
  const normalized = rawPath.replaceAll("\\", "/").replace(/^\.\/+/, "").replace(/\/+$/, "");
  return normalized === "" ? "." : normalized;
}

export function buildPolicyDecision(requested: Capability[], granted: Capability[], approvalRequired: boolean, reason: string): PolicyDecision {
  return {
    capabilities_requested: requested,
    capabilities_granted: granted,
    approval_required: approvalRequired,
    reason,
  };
}

export function policyToRisk(policy: PolicyDecision): Risk {
  return {
    approval_required: policy.approval_required,
    reason: policy.reason,
  };
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
