/**
 * 会话续期调度。
 *
 * Archery（Django）按最后活动时间计算会话有效期，长时间不操作就会失效。
 * sidecar 侧的心跳接口会发一个无副作用的登录态 GET，因此 UI 只需要按节奏触发检查：
 * 检查间隔比 sidecar 的 keep_alive_secs 更密，由后端节流决定是否真正打到 Archery。
 */

/** UI 检查心跳的间隔：比后端默认心跳（120 秒）更密，避免刚好踩在过期边界。 */
export const SESSION_CHECK_INTERVAL_MS = 60_000;

/** 连续失败时的退避上限，避免网络长时间不可用时无谓地堆积请求。 */
export const SESSION_RETRY_MAX_MS = 5 * 60_000;

/**
 * 是否该触发一次心跳。
 * `lastAt` 为 0 表示从未检查过（刚连上时立即补一次，把会话活动时间推到最新）。
 */
export function shouldProbeSession(
  lastAt: number,
  now: number,
  intervalMs: number = SESSION_CHECK_INTERVAL_MS,
): boolean {
  if (!Number.isFinite(intervalMs) || intervalMs <= 0) return false;
  if (!Number.isFinite(lastAt) || lastAt <= 0) return true;
  return now - lastAt >= intervalMs;
}

/**
 * 下一次心跳的等待时间：成功时按基础间隔，连续失败按 1x / 2x / 4x 退避到上限。
 * `failures` 是「连续失败次数」，成功一次就归零。
 */
export function sessionRetryDelay(
  failures: number,
  baseMs: number = SESSION_CHECK_INTERVAL_MS,
): number {
  const base = Math.max(1, baseMs);
  const count = Number.isFinite(failures) ? Math.max(0, Math.floor(failures)) : 0;
  if (count <= 1) return base;
  return Math.min(base * 2 ** (count - 1), SESSION_RETRY_MAX_MS);
}

/** UI 检查间隔的下限：配置再小也不至于把心跳打成轮询。 */
export const SESSION_PROBE_MIN_MS = 15_000;

/**
 * 按 sidecar 回报的心跳间隔对齐 UI 检查节奏：取一半，保证真正探测落在
 * 会话过期之前；默认 120 秒 → 60 秒，配置 30 秒 → 15 秒，配置 600 秒 → 300 秒。
 */
export function probeDelayForInterval(intervalSec: number): number {
  if (!Number.isFinite(intervalSec) || intervalSec <= 0) return SESSION_CHECK_INTERVAL_MS;
  return Math.max(SESSION_PROBE_MIN_MS, Math.floor((intervalSec * 1000) / 2));
}

/** 心跳失败里属于「会话真的没了、必须人工验证」的两类。 */
export function isSessionExpiredKind(kind: string): boolean {
  return kind === "auth_required" || kind === "twofa_required";
}
