import { describe, expect, it } from "vitest";
import {
  SESSION_CHECK_INTERVAL_MS,
  SESSION_PROBE_MIN_MS,
  SESSION_RETRY_MAX_MS,
  isSessionExpiredKind,
  probeDelayForInterval,
  sessionRetryDelay,
  shouldProbeSession,
} from "./session";

describe("shouldProbeSession", () => {
  it("从未检查过时立即探测", () => {
    expect(shouldProbeSession(0, Date.now())).toBe(true);
  });

  it("未到间隔时不探测", () => {
    const now = 1_000_000;
    expect(shouldProbeSession(now - SESSION_CHECK_INTERVAL_MS + 1, now)).toBe(false);
  });

  it("刚好到间隔就探测", () => {
    const now = 1_000_000;
    expect(shouldProbeSession(now - SESSION_CHECK_INTERVAL_MS, now)).toBe(true);
  });

  it("间隔被配置为 0 或负数时不探测", () => {
    expect(shouldProbeSession(0, 1000, 0)).toBe(false);
    expect(shouldProbeSession(0, 1000, -1)).toBe(false);
  });

  it("时钟回拨（now 小于 lastAt）不探测，避免连发请求", () => {
    expect(shouldProbeSession(2000, 1000)).toBe(false);
  });
});

describe("sessionRetryDelay", () => {
  it("成功或首次失败都按基础间隔", () => {
    expect(sessionRetryDelay(0)).toBe(SESSION_CHECK_INTERVAL_MS);
    expect(sessionRetryDelay(1)).toBe(SESSION_CHECK_INTERVAL_MS);
  });

  it("连续失败按指数退避", () => {
    expect(sessionRetryDelay(2)).toBe(SESSION_CHECK_INTERVAL_MS * 2);
    expect(sessionRetryDelay(3)).toBe(SESSION_CHECK_INTERVAL_MS * 4);
  });

  it("退避不超过上限", () => {
    expect(sessionRetryDelay(20)).toBe(SESSION_RETRY_MAX_MS);
    expect(sessionRetryDelay(20, 10 * 60_000)).toBe(SESSION_RETRY_MAX_MS);
  });

  it("非法失败次数按首次失败处理", () => {
    expect(sessionRetryDelay(-3)).toBe(SESSION_CHECK_INTERVAL_MS);
    expect(sessionRetryDelay(Number.NaN)).toBe(SESSION_CHECK_INTERVAL_MS);
  });
});

describe("probeDelayForInterval", () => {
  it("默认 120 秒按一半对齐成 60 秒", () => {
    expect(probeDelayForInterval(120)).toBe(60_000);
  });

  it("配置更小时跟着缩短，但不下探到轮询", () => {
    expect(probeDelayForInterval(30)).toBe(15_000);
    expect(probeDelayForInterval(10)).toBe(SESSION_PROBE_MIN_MS);
  });

  it("配置更大时拉长，减少无谓请求", () => {
    expect(probeDelayForInterval(600)).toBe(300_000);
  });

  it("非法或关闭时退回默认检查间隔", () => {
    expect(probeDelayForInterval(0)).toBe(SESSION_CHECK_INTERVAL_MS);
    expect(probeDelayForInterval(Number.NaN)).toBe(SESSION_CHECK_INTERVAL_MS);
  });
});

describe("isSessionExpiredKind", () => {
  it("只有需要人工验证的两类才算会话过期", () => {
    expect(isSessionExpiredKind("auth_required")).toBe(true);
    expect(isSessionExpiredKind("twofa_required")).toBe(true);
    expect(isSessionExpiredKind("network")).toBe(false);
    expect(isSessionExpiredKind("timeout")).toBe(false);
    expect(isSessionExpiredKind("not_connected")).toBe(false);
  });
});
