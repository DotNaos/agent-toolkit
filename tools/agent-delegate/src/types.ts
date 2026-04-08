export type Mode = "advisory" | "guarded_execution";
export type Status = "completed" | "failed" | "blocked" | "timed_out";
export type AdapterId = "gemini" | "claude" | "copilot" | "codex";
export type Capability = "read" | "write" | "exec" | "network" | "git";
export type ContextType = "inline" | "file" | "snippet" | "glob" | "search_results" | "command_output" | "git_diff";
export type ArtifactKind = "created_file" | "updated_file" | "deleted_file" | "report";

export type ContextItem = {
  type?: ContextType;
  label?: string;
  text?: string;
  path?: string;
  start_line?: number;
  end_line?: number;
};

export type JSONSchema = {
  type?: string;
  properties?: Record<string, JSONSchema>;
  required?: string[];
  items?: JSONSchema;
  enum?: unknown[];
  additionalProperties?: boolean;
};

export type ResponseFormat = {
  type?: "text" | "json_schema";
  schema?: JSONSchema;
};

export type Request = {
  adapter: AdapterId;
  model?: string;
  task: string;
  mode?: Mode;
  cwd?: string;
  context?: ContextItem[];
  timeout_sec?: number;
  metadata?: Record<string, unknown>;
  capabilities?: Capability[];
  allowed_paths?: string[];
  response_format?: ResponseFormat;
};

export type Risk = {
  approval_required: boolean;
  reason?: string;
};

export type PolicyDecision = {
  capabilities_requested: Capability[];
  capabilities_granted: Capability[];
  approval_required: boolean;
  reason: string;
};

export type ChangeSet = {
  created: string[];
  updated: string[];
  deleted: string[];
};

export type Artifact = {
  path: string;
  kind: ArtifactKind;
  content?: string;
};

export type Result = {
  status: Status;
  adapter: string;
  mode: Mode;
  final_text?: string;
  stdout?: string;
  stderr?: string;
  exit_code?: number;
  duration_ms: number;
  artifacts?: Artifact[];
  structured_output?: unknown;
  changes?: ChangeSet;
  policy?: PolicyDecision;
  risk: Risk;
};

export type DefaultsConfig = {
  timeout_sec?: number;
  max_timeout_sec?: number;
};

export type PolicyConfig = {
  default_capabilities?: Capability[];
  approval_required_for?: Capability[];
  allow_heuristic_fallback?: boolean;
};

export type ModelConfig = {
  id: string;
  label?: string;
  aliases?: string[];
  multiplier?: number;
};

export type AdapterConfig = {
  enabled: boolean;
  command: string;
  args?: string[];
  timeout_sec?: number;
  supports_guarded_execution: boolean;
  default_model?: string;
  models?: ModelConfig[];
  supported_capabilities?: Capability[];
};

export type Config = {
  defaults?: DefaultsConfig;
  policy?: PolicyConfig;
  adapters: Record<string, AdapterConfig>;
};

export type LoadedConfig = {
  defaults: Required<DefaultsConfig>;
  policy: Required<PolicyConfig>;
  adapters: Record<string, AdapterConfig>;
};

export type BridgeRequest = {
  request: Request;
  approval_granted?: boolean;
};

export type NormalizedRequest = {
  adapter: AdapterId;
  model: string;
  task: string;
  mode: Mode;
  cwd: string;
  context: Required<ContextItem>[];
  timeout_sec: number;
  metadata: Record<string, unknown>;
  capabilities: Capability[];
  allowed_paths: string[];
  response_format: Required<ResponseFormat>;
};

export type HeuristicAssessment = {
  approval_required: boolean;
  reason: string;
};

export type PolicyEvaluation = {
  decision: PolicyDecision;
  blocked_reason: string;
};

export type WorkspaceSnapshot = Map<string, string | null>;

export const adapters: AdapterId[] = ["gemini", "claude", "copilot", "codex"];
export const capabilities: Capability[] = ["read", "write", "exec", "network", "git"];
export const guardedExecutionCapabilities: Capability[] = ["read", "write", "exec", "git"];
export const riskyKeywords = ["delete", "deploy", "publish", "run migration", "drop", "kill", "charge", "send email"];
export const safeKeywords = ["read", "analyze", "inspect", "list", "summarize", "explain", "review", "show", "design"];
export const internalCodexOutput = ".delegate-codex-last-message.txt";
export const schemaReportPath = ".delegate-structured-output-report.txt";
export const maxArtifactBytes = 64 * 1024;
export const ignoredArtifactDirs = new Set([".git", "node_modules", ".next", "dist", "build", "coverage", ".turbo", ".cache"]);
