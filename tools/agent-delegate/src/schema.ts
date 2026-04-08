import { Artifact, JSONSchema, NormalizedRequest, Status, schemaReportPath } from "./types";

export async function finalizeOutput(request: NormalizedRequest, finalText: string, stdout: string, artifacts: Artifact[]) {
  if (request.response_format.type !== "json_schema") {
    return { status: undefined as Status | undefined, structured_output: undefined, artifacts };
  }

  const candidate = finalText || stdout.trim();
  let parsed: unknown;
  try {
    parsed = JSON.parse(candidate);
  } catch (error) {
    return {
      status: "failed" as Status,
      structured_output: undefined,
      artifacts: appendReportArtifact(artifacts, `Structured output is not valid JSON.\n\n${formatError(error)}\n\nRaw output:\n${candidate}`),
    };
  }

  const validationError = validateJSONSchema(parsed, request.response_format.schema, "$");
  if (validationError) {
    return {
      status: "failed" as Status,
      structured_output: undefined,
      artifacts: appendReportArtifact(artifacts, `Structured output does not satisfy schema.\n\n${validationError}\n\nRaw output:\n${candidate}`),
    };
  }

  return {
    status: undefined as Status | undefined,
    structured_output: parsed,
    artifacts,
  };
}

export function validateJSONSchema(value: unknown, schema: JSONSchema, pointer: string): string {
  if (!schema || Object.keys(schema).length === 0) {
    return "";
  }

  if (Array.isArray(schema.enum) && !schema.enum.some((candidate) => deepEqual(candidate, value))) {
    return `${pointer} must match one of enum values`;
  }

  switch (schema.type) {
    case undefined:
      break;
    case "object":
      if (!isPlainObject(value)) {
        return `${pointer} must be an object`;
      }
      break;
    case "array":
      if (!Array.isArray(value)) {
        return `${pointer} must be an array`;
      }
      break;
    case "string":
      return typeof value === "string" ? "" : `${pointer} must be a string`;
    case "number":
      return typeof value === "number" && !Number.isNaN(value) ? "" : `${pointer} must be a number`;
    case "integer":
      return typeof value === "number" && Number.isInteger(value) ? "" : `${pointer} must be an integer`;
    case "boolean":
      return typeof value === "boolean" ? "" : `${pointer} must be a boolean`;
    case "null":
      return value === null ? "" : `${pointer} must be null`;
    default:
      return `${pointer} has unsupported schema type ${JSON.stringify(schema.type)}`;
  }

  if (schema.type === "object" && isPlainObject(value)) {
    for (const key of schema.required ?? []) {
      if (!(key in value)) {
        return `${pointer}.${key} is required`;
      }
    }
    const properties = schema.properties ?? {};
    for (const [key, childSchema] of Object.entries(properties)) {
      if (!(key in value)) {
        continue;
      }
      const childError = validateJSONSchema((value as Record<string, unknown>)[key], childSchema, `${pointer}.${key}`);
      if (childError) {
        return childError;
      }
    }
    if (schema.additionalProperties === false) {
      for (const key of Object.keys(value)) {
        if (!(key in properties)) {
          return `${pointer}.${key} is not allowed`;
        }
      }
    }
  }

  if (schema.type === "array" && Array.isArray(value) && schema.items) {
    for (let index = 0; index < value.length; index += 1) {
      const childError = validateJSONSchema(value[index], schema.items, `${pointer}[${index}]`);
      if (childError) {
        return childError;
      }
    }
  }

  return "";
}

function appendReportArtifact(artifacts: Artifact[], content: string) {
  return [...artifacts, { path: schemaReportPath, kind: "report" as const, content }];
}

function deepEqual(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function formatError(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
