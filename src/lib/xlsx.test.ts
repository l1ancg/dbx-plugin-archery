import { describe, expect, it } from "vitest";
import { buildXlsx, exportFilename, XlsxTooLargeError, type XlsxSheet } from "./xlsx";

function u16(data: Uint8Array, at: number): number {
  return data[at] | (data[at + 1] << 8);
}

function u32(data: Uint8Array, at: number): number {
  return (
    (data[at] | (data[at + 1] << 8) | (data[at + 2] << 16) | (data[at + 3] << 24)) >>> 0
  );
}

/** 解析 store（无压缩）zip 的本地文件头，返回 部件名 → 内容。 */
function readStoreZip(data: Uint8Array): Map<string, Uint8Array> {
  const entries = new Map<string, Uint8Array>();
  let offset = 0;
  while (offset + 4 <= data.length) {
    const signature = u32(data, offset);
    // 中央目录 / EOCD 之后不再有本地文件头。
    if (signature === 0x02014b50 || signature === 0x06054b50) break;
    expect(signature).toBe(0x04034b50);
    expect(u16(data, offset + 8)).toBe(0); // 压缩方式必须是 store
    const size = u32(data, offset + 18);
    const nameLength = u16(data, offset + 26);
    const extraLength = u16(data, offset + 28);
    const nameStart = offset + 30;
    const name = new TextDecoder().decode(data.subarray(nameStart, nameStart + nameLength));
    const start = nameStart + nameLength + extraLength;
    entries.set(name, data.subarray(start, start + size));
    offset = start + size;
  }
  return entries;
}

function decode(bytes: Uint8Array | undefined): string {
  return new TextDecoder().decode(bytes ?? new Uint8Array());
}

function sheetXml(sheet: XlsxSheet): string {
  return decode(readStoreZip(buildXlsx(sheet)).get("xl/worksheets/sheet1.xml"));
}

describe("buildXlsx 包结构", () => {
  it("包含 xlsx 必需的六个部件", () => {
    const parts = readStoreZip(buildXlsx({ columns: ["id"], rows: [[1]] }));
    expect([...parts.keys()].sort()).toEqual(
      [
        "[Content_Types].xml",
        "_rels/.rels",
        "xl/_rels/workbook.xml.rels",
        "xl/styles.xml",
        "xl/workbook.xml",
        "xl/worksheets/sheet1.xml",
      ].sort(),
    );
  });
});

describe("buildXlsx 单元格", () => {
  it("表头用加粗样式，数据行按列序写入", () => {
    const xml = sheetXml({ columns: ["id", "name"], rows: [[1, "alpha"]] });
    // 表头 A1 是 inlineStr 且引用 styles.xml 里第 2 个（加粗）格式。
    expect(xml).toMatch(/<c r="A1"[^>]*s="1"[^>]*t="inlineStr"/);
    expect(xml).toContain("<t>id</t>");
    expect(xml).toContain("<t>name</t>");
    expect(xml).toContain('<c r="A2"><v>1</v></c>');
    expect(xml).toMatch(/<c r="B2"[^>]*t="inlineStr"[^>]*><is><t>alpha<\/t><\/is><\/c>/);
  });

  it("数字写数值单元格；超 15 位整数与 NaN/Infinity 转文本", () => {
    const xml = sheetXml({
      columns: ["i", "d", "big", "nan", "inf"],
      rows: [[42, 3.5, 1e16, Number.NaN, Number.POSITIVE_INFINITY]],
    });
    expect(xml).toContain('<c r="A2"><v>42</v></c>');
    expect(xml).toContain('<c r="B2"><v>3.5</v></c>');
    expect(xml).toContain('<t>10000000000000000</t>');
    expect(xml).toContain("<t>NaN</t>");
    expect(xml).toContain("<t>Infinity</t>");
  });

  it("布尔写 b 类型，NULL 不写单元格但保留整行", () => {
    const xml = sheetXml({ columns: ["t", "f", "n"], rows: [[true, false, null]] });
    expect(xml).toContain('<c r="A2" t="b"><v>1</v></c>');
    expect(xml).toContain('<c r="B2" t="b"><v>0</v></c>');
    expect(xml).not.toContain('r="C2"');
  });

  it("文本转义特殊字符、剔除非法控制字符并保留换行", () => {
    const xml = sheetXml({
      columns: ["v"],
      rows: [['<a & "b">'], ["x\u0001y"], ["l1\nl2"]],
    });
    expect(xml).toContain("<t>&lt;a &amp; &quot;b&quot;&gt;</t>");
    expect(xml).toContain("<t>xy</t>");
    expect(xml).toContain("<t>l1\nl2</t>");
  });

  it("列号超过 Z 后进位", () => {
    const columns = Array.from({ length: 28 }, (_, index) => `c${index}`);
    const xml = sheetXml({ columns, rows: [] });
    expect(xml).toContain('<c r="AB1"');
  });

  it("单格文本按 Excel 上限 32767 截断", () => {
    const xml = sheetXml({ columns: ["v"], rows: [["x".repeat(40000)]] });
    const match = /<t>(x+)<\/t>/.exec(xml);
    expect(match?.[1].length).toBe(32767);
  });
});

describe("buildXlsx 工作表", () => {
  it("首行冻结、开启自动筛选、dimension 覆盖全部数据", () => {
    const xml = sheetXml({
      columns: ["a", "b", "c"],
      rows: [
        [1, 2, 3],
        [4, 5, 6],
      ],
    });
    expect(xml).toContain('<dimension ref="A1:C3"/>');
    expect(xml).toContain(
      '<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>',
    );
    expect(xml).toContain('<autoFilter ref="A1:C3"/>');
  });

  it("工作表名替换非法字符并截断到 31 字符", () => {
    const invalid = readStoreZip(
      buildXlsx({ name: "a[]:*?/\\b", columns: ["v"], rows: [] }),
    );
    expect(decode(invalid.get("xl/workbook.xml"))).toContain('<sheet name="a_______b"');

    const tooLong = readStoreZip(
      buildXlsx({ name: "x".repeat(40), columns: ["v"], rows: [] }),
    );
    expect(decode(tooLong.get("xl/workbook.xml"))).toContain(
      `<sheet name="${"x".repeat(31)}"`,
    );
  });
});

describe("导出边界", () => {
  it("超过限定字节数抛 XlsxTooLargeError", () => {
    let caught: unknown;
    try {
      buildXlsx({ columns: ["a"], rows: [[1]] }, { maxBytes: 256 });
    } catch (error) {
      caught = error;
    }
    expect(caught).toBeInstanceOf(XlsxTooLargeError);
    expect((caught as XlsxTooLargeError).bytes).toBeGreaterThan(256);
  });

  it("文件名带时间戳且符合 sidecar 的落盘白名单", () => {
    const at = new Date(2026, 10, 9, 15, 30, 12);
    expect(exportFilename(at)).toBe("query-result-20261109-153012.xlsx");
    expect(exportFilename(at)).toMatch(/^[A-Za-z0-9][A-Za-z0-9._-]*$/);
  });
});

describe("buildXlsx zip 目录", () => {
  // 真实解析器（Python zipfile / Excel）读的是中央目录与 EOCD：
  // 大小或偏移差一个字节就会 BadZipFile，而只走本地文件头的解析器看不出来。
  it("EOCD 记录的条目数、中央目录大小与偏移自洽", () => {
    const bytes = buildXlsx({ columns: ["id", "name"], rows: [[1, "alpha"]] });
    const eocd = bytes.length - 22;
    expect(u32(bytes, eocd)).toBe(0x06054b50);
    expect(u16(bytes, eocd + 8)).toBe(6);
    expect(u16(bytes, eocd + 10)).toBe(6);

    const size = u32(bytes, eocd + 12);
    const offset = u32(bytes, eocd + 16);
    expect(u32(bytes, offset)).toBe(0x02014b50);
    expect(offset + size).toBe(eocd);

    // 逐条走完中央目录，必须正好停在 EOCD 上。
    let cursor = offset;
    for (let index = 0; index < 6; index += 1) {
      expect(u32(bytes, cursor)).toBe(0x02014b50);
      const nameLength = u16(bytes, cursor + 28);
      const extraLength = u16(bytes, cursor + 30);
      const commentLength = u16(bytes, cursor + 32);
      cursor += 46 + nameLength + extraLength + commentLength;
    }
    expect(cursor).toBe(eocd);
  });
});
