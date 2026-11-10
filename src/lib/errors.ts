import { PluginError, normalizeError } from "../api";
import { localizedError } from "../i18n";

export function describeError(error: unknown): string {
  const normalized = error instanceof PluginError ? error : normalizeError(error);
  return localizedError(normalized.kind, normalized.message);
}

export function errorKind(error: unknown): string {
  return error instanceof PluginError ? error.kind : "internal";
}
