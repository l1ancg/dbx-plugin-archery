import { describe, expect, it } from "vitest";
import { ANNOUNCEMENT_TTL_MS, shouldRefreshAnnouncement } from "./announcement";

describe("shouldRefreshAnnouncement", () => {
  it("首次加载时总是刷新", () => {
    expect(shouldRefreshAnnouncement(0, 1_700_000_000_000)).toBe(true);
  });

  it("TTL 内复用已有公告，不重复打接口", () => {
    expect(shouldRefreshAnnouncement(1_000, 1_000 + ANNOUNCEMENT_TTL_MS - 1)).toBe(false);
  });

  it("到达 TTL 后重新拉取", () => {
    expect(shouldRefreshAnnouncement(1_000, 1_000 + ANNOUNCEMENT_TTL_MS)).toBe(true);
  });

  it("系统时钟回拨时按过期处理，避免公告永久不更新", () => {
    expect(shouldRefreshAnnouncement(10_000, 5_000)).toBe(true);
  });

  it("默认 TTL 为 60 秒", () => {
    expect(ANNOUNCEMENT_TTL_MS).toBe(60_000);
  });
});
