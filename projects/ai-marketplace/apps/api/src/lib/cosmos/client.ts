/**
 * In-memory store — replaces @azure/cosmos for local/dev usage.
 * Exports the same `getContainer` / `CONTAINERS` interface so call-sites
 * remain unchanged. Data is scoped to the current process lifetime.
 */

/* eslint-disable @typescript-eslint/no-explicit-any */

// ── types ────────────────────────────────────────────────────────────────────

interface QueryParameter {
  name: string; // e.g. "@tenantId"
  value: unknown;
}

interface QuerySpec {
  query: string;
  parameters?: QueryParameter[];
}

// ── in-memory store ──────────────────────────────────────────────────────────

const store = new Map<string, Map<string, any>>();

function getStore(containerName: string): Map<string, any> {
  if (!store.has(containerName)) store.set(containerName, new Map());
  return store.get(containerName)!;
}

// ── minimal SQL WHERE parser ─────────────────────────────────────────────────

/**
 * Parses a very limited subset of Cosmos SQL used in this codebase:
 *   WHERE c.field = @param
 *   WHERE c.field IN ('a','b','c')
 *   AND  c.field = @param
 * Returns a filter predicate over items.
 */
function buildPredicate(
  query: string,
  params: QueryParameter[]
): (item: any) => boolean {
  const paramMap = new Map(params.map((p) => [p.name, p.value]));

  // Extract everything after WHERE (case-insensitive)
  const whereMatch = query.match(/WHERE\s+(.+)$/is);
  if (!whereMatch) return () => true;

  const wherePart = whereMatch[1]
    // Strip ORDER BY clause before parsing conditions
    .replace(/ORDER\s+BY\s+.+$/is, "")
    .trim();

  const conditions = wherePart.split(/\bAND\b/i).map((s) => s.trim());

  const checks = conditions.map((cond): ((item: any) => boolean) => {
    // c.field = @param
    const eqMatch = cond.match(/^c\.(\w+)\s*=\s*(@\w+)$/i);
    if (eqMatch) {
      const [, field, paramName] = eqMatch;
      const expected = paramMap.get(paramName);
      return (item) => item[field] === expected;
    }

    // c.field IN ('v1','v2',...)
    const inMatch = cond.match(/^c\.(\w+)\s+IN\s*\(([^)]+)\)/i);
    if (inMatch) {
      const [, field, rawList] = inMatch;
      const values = rawList
        .split(",")
        .map((s) => s.trim().replace(/^['"]|['"]$/g, ""));
      return (item) => values.includes(String(item[field] ?? ""));
    }

    // Unknown condition — pass through
    return () => true;
  });

  return (item) => checks.every((fn) => fn(item));
}

// ── fake Container ────────────────────────────────────────────────────────────

class InMemoryContainer {
  constructor(private readonly containerName: string) {}

  /** Mimic CosmosDB Container.items */
  readonly items = {
    create: async <T = any>(body: T): Promise<{ resource: T }> => {
      const b = body as any;
      const id = String(b.id ?? Math.random().toString(36).slice(2));
      const record = { ...b, id } as T;
      getStore(this.containerName).set(id, record);
      return { resource: record };
    },

    upsert: async <T = any>(body: T): Promise<{ resource: T }> => {
      const b = body as any;
      const id = String(b.id ?? Math.random().toString(36).slice(2));
      const record = { ...b, id } as T;
      getStore(this.containerName).set(id, record);
      return { resource: record };
    },

    query: <T = any>(spec: QuerySpec | string) => {
      const querySpec: QuerySpec =
        typeof spec === "string" ? { query: spec, parameters: [] } : spec;
      const predicate = buildPredicate(
        querySpec.query,
        querySpec.parameters ?? []
      );
      const name = this.containerName;
      return {
        fetchAll: async (): Promise<{ resources: T[] }> => {
          const all = Array.from(getStore(name).values());
          return { resources: all.filter(predicate) as T[] };
        },
      };
    },
  };

  /** Mimic CosmosDB Container.item(id, partitionKey) */
  item(id: string, _partitionKey?: string) {
    const containerName = this.containerName;
    return {
      delete: async (): Promise<any> => {
        getStore(containerName).delete(id);
        return {};
      },
      read: async <T = any>(): Promise<{ resource: T | undefined }> => {
        const resource = getStore(containerName).get(id) as T | undefined;
        return { resource };
      },
    };
  }
}

// ── public API ────────────────────────────────────────────────────────────────

export async function getContainer(
  containerName: string
): Promise<InMemoryContainer> {
  return new InMemoryContainer(containerName);
}

export const CONTAINERS = {
  ASSETS: "assets",
  PUBLISHERS: "publishers",
  SUBMISSIONS: "submissions",
  WORKFLOWS: "workflows",
  AUDIT_LOG: "audit-log",
  RATINGS: "ratings",
  PROJECTS: "projects",
  VERSION_PINS: "version-pins",
  SESSIONS: "sessions",
  USER_CONFIG: "user-config",
} as const;
