import { readFile } from "node:fs/promises";

import { AdapterConfig, Capability, LoadedConfig } from "./types";
import { normalizeCapabilities } from "./request";
import { resolvePath } from "./workspace";

export async function loadConfig(configPath: string): Promise<LoadedConfig> {
  const data = await readFile(resolvePath(configPath), "utf8");
  const config = JSON.parse(data) as {
    defaults?: { timeout_sec?: number; max_timeout_sec?: number };
    policy?: { default_capabilities?: Capability[]; approval_required_for?: Capability[]; allow_heuristic_fallback?: boolean };
    adapters?: Record<string, AdapterConfig>;
  };

  const normalized: LoadedConfig = {
    defaults: {
      timeout_sec: config.defaults?.timeout_sec && config.defaults.timeout_sec > 0 ? config.defaults.timeout_sec : 120,
      max_timeout_sec: config.defaults?.max_timeout_sec && config.defaults.max_timeout_sec > 0 ? config.defaults.max_timeout_sec : 600,
    },
    policy: {
      default_capabilities: normalizeCapabilities(config.policy?.default_capabilities, "config policy.default_capabilities", ["read"]),
      approval_required_for: normalizeCapabilities(config.policy?.approval_required_for, "config policy.approval_required_for", ["write", "exec", "network", "git"]),
      allow_heuristic_fallback: config.policy?.allow_heuristic_fallback ?? true,
    },
    adapters: config.adapters ?? {},
  };

  if (Object.keys(normalized.adapters).length === 0) {
    throw new Error("config must define at least one adapter");
  }

  for (const [id, adapter] of Object.entries(normalized.adapters)) {
    if (!adapter.supported_capabilities || adapter.supported_capabilities.length === 0) {
      adapter.supported_capabilities = ["read", "write", "exec", "network", "git"];
    } else {
      adapter.supported_capabilities = normalizeCapabilities(adapter.supported_capabilities, `adapter ${JSON.stringify(id)} supported_capabilities`);
    }
    if (!adapter.enabled) {
      continue;
    }
    if (!adapter.command?.trim()) {
      throw new Error(`adapter ${JSON.stringify(id)} missing command`);
    }
    for (const model of adapter.models ?? []) {
      if (!String(model.id || "").trim()) {
        throw new Error(`adapter ${JSON.stringify(id)} contains model with empty id`);
      }
    }
  }

  return normalized;
}

export function listEnabledAdapters(config: LoadedConfig) {
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
      supported_capabilities: adapter.supported_capabilities ?? ["read", "write", "exec", "network", "git"],
      models: (adapter.models ?? []).map((model) => ({
        ...model,
        qualified_id: `${id}/${model.id}`,
      })),
    }));
}

export function resolveAdapterCommand(adapterId: string, adapter: AdapterConfig) {
  const envName = `AGENT_DELEGATE_${adapterId.toUpperCase().replace(/-/g, "_")}_COMMAND`;
  return String(process.env[envName] || adapter.command);
}
