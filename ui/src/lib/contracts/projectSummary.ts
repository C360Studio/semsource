export type PredicateRole =
  "identity" | "content" | "location" | "relationship" | "metric" | "metadata";
export interface TypeCount {
  type: string;
  count: number;
}
export interface DomainSummary {
  domain: string;
  entity_count: number;
  types: TypeCount[];
  sources: string[];
}
export interface PredicateDescriptor {
  name: string;
  description: string;
  data_type: string;
  role: PredicateRole;
}
export interface PredicateSchema {
  source_type: string;
  predicates: PredicateDescriptor[];
}
export interface ProjectSummary {
  namespace: string;
  phase: "seeding" | "ready" | "degraded";
  entity_id_format: string;
  total_entities: number;
  domains: DomainSummary[];
  predicates: PredicateSchema[];
  timestamp: string;
}
function record(v: unknown): Record<string, unknown> | null {
  return typeof v === "object" && v !== null && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : null;
}
function integer(v: unknown, name: string): number {
  if (typeof v !== "number" || !Number.isSafeInteger(v) || v < 0)
    throw new Error(`Invalid project summary: malformed ${name}`);
  return v;
}
function array(v: unknown, name: string): unknown[] {
  if (v === null) return [];
  if (!Array.isArray(v))
    throw new Error(`Invalid project summary: malformed ${name}`);
  return v;
}
function text(v: unknown, name: string): string {
  if (typeof v !== "string")
    throw new Error(`Invalid project summary: malformed ${name}`);
  return v;
}
function rfc3339(v: unknown): string {
  const textValue = text(v, "timestamp");
  if (
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      textValue,
    ) ||
    Number.isNaN(Date.parse(textValue))
  )
    throw new Error("Invalid project summary: malformed timestamp");
  return textValue;
}
export function parseProjectSummary(value: unknown): ProjectSummary {
  const item = record(value);
  if (!item) throw new Error("Invalid project summary: expected an object");
  if (!["seeding", "ready", "degraded"].includes(String(item.phase)))
    throw new Error("Invalid project summary: malformed phase");
  const domains = array(item.domains, "domains").map((v) => {
    const d = record(v);
    if (!d) throw new Error("Invalid project summary: malformed domain");
    return {
      domain: text(d.domain, "domain"),
      entity_count: integer(d.entity_count, "domain entity_count"),
      types: array(d.types, "types").map((t) => {
        const x = record(t);
        if (!x) throw new Error("Invalid project summary: malformed type");
        return {
          type: text(x.type, "type"),
          count: integer(x.count, "type count"),
        };
      }),
      sources: array(d.sources, "sources").map((s) => text(s, "source")),
    };
  });
  const roles = [
    "identity",
    "content",
    "location",
    "relationship",
    "metric",
    "metadata",
  ];
  const predicates = array(item.predicates, "predicates").map((v) => {
    const p = record(v);
    if (!p)
      throw new Error("Invalid project summary: malformed predicate schema");
    return {
      source_type: text(p.source_type, "source_type"),
      predicates: array(p.predicates, "predicates").map((q) => {
        const x = record(q);
        if (!x || !roles.includes(String(x.role)))
          throw new Error("Invalid project summary: malformed predicate role");
        return {
          name: text(x.name, "predicate name"),
          description: text(x.description, "predicate description"),
          data_type: text(x.data_type, "predicate data_type"),
          role: x.role as PredicateRole,
        };
      }),
    };
  });
  return {
    namespace: text(item.namespace, "namespace"),
    phase: item.phase as ProjectSummary["phase"],
    entity_id_format: text(item.entity_id_format, "entity_id_format"),
    total_entities: integer(item.total_entities, "total_entities"),
    domains,
    predicates,
    timestamp: rfc3339(item.timestamp),
  };
}
