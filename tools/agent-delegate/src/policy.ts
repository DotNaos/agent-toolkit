import { AdapterConfig, HeuristicAssessment, Mode, NormalizedRequest, PolicyConfig, PolicyEvaluation, riskyKeywords, safeKeywords } from "./types";
import { buildPolicyDecision } from "./request";

export function evaluatePolicy(request: NormalizedRequest, policy: Required<PolicyConfig>, adapter: AdapterConfig, approvalGranted: boolean): PolicyEvaluation {
  const requested = [...request.capabilities];
  const supported = new Set(adapter.supported_capabilities ?? []);
  const unsupported = requested.filter((capability) => !supported.has(capability));
  if (unsupported.length > 0) {
    const reason = `adapter does not support capabilities: ${unsupported.join(", ")}`;
    return {
      decision: buildPolicyDecision(requested, [], false, reason),
      blocked_reason: reason,
    };
  }

  const capabilitiesRequiringApproval = requested.filter((capability) => policy.approval_required_for.includes(capability));
  const heuristic = policy.allow_heuristic_fallback ? assessHeuristicRisk(request.task, request.metadata, request.mode) : { approval_required: false, reason: "heuristic fallback disabled" };
  const approvalRequired = capabilitiesRequiringApproval.length > 0 || heuristic.approval_required;

  let reason = "";
  if (capabilitiesRequiringApproval.length > 0) {
    reason = `requested capabilities require approval: ${capabilitiesRequiringApproval.join(", ")}`;
  } else if (heuristic.approval_required) {
    reason = `heuristic fallback: ${heuristic.reason}`;
  } else {
    reason = `capabilities allowed without approval: ${requested.join(", ")}`;
  }

  if (approvalRequired && !approvalGranted) {
    return {
      decision: buildPolicyDecision(requested, [], true, reason),
      blocked_reason: reason,
    };
  }

  return {
    decision: buildPolicyDecision(requested, requested, approvalRequired, reason),
    blocked_reason: "",
  };
}

export function assessHeuristicRisk(task: string, metadata: Record<string, unknown>, mode: Mode): HeuristicAssessment {
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
