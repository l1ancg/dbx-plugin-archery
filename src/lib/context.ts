function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

/**
 * 宿主不同版本把连接 id 放在 context.connectionId、context.connection.id
 * 或 context.id，统一在这里兜底。
 */
export function resolveConnectionId(context: Record<string, unknown> | undefined | null): string {
  if (!context) return "";
  const direct = text(context.connectionId);
  if (direct) return direct;

  const connection = context.connection;
  if (connection && typeof connection === "object") {
    const nested = text((connection as Record<string, unknown>).id);
    if (nested) return nested;
  }

  return text(context.id);
}

export function resolveConnectionName(
  context: Record<string, unknown> | undefined | null,
  fallback = "Archery",
): string {
  if (!context) return fallback;
  const direct = text(context.connectionName) || text(context.name);
  if (direct) return direct;

  const connection = context.connection;
  if (connection && typeof connection === "object") {
    const nested = text((connection as Record<string, unknown>).name);
    if (nested) return nested;
  }
  return fallback;
}
