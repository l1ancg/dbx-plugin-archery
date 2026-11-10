/** 公告刷新 TTL：切页时距上次拉取超过 60 秒才重新请求 Archery 首页。 */
export const ANNOUNCEMENT_TTL_MS = 60_000;

/**
 * 判断是否需要重新拉取公告。
 * lastLoadedAt 为 0 表示从未拉取；系统时钟回拨（now < lastLoadedAt）按过期处理，
 * 否则公告可能因为一次时间跳变永久不再刷新。
 */
export function shouldRefreshAnnouncement(
  lastLoadedAt: number,
  now: number,
  ttlMs = ANNOUNCEMENT_TTL_MS,
): boolean {
  if (lastLoadedAt <= 0) return true;
  if (now < lastLoadedAt) return true;
  return now - lastLoadedAt >= ttlMs;
}
