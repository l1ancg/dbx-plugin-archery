import type { Cell } from "../types";

/** xlsx 的 MIME，浏览器回退下载时用。 */
export const XLSX_MIME =
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

/**
 * 导出上限。sidecar 单条 JSON 消息上限 8MB（SDK 的 maxJSONBytes），
 * base64 会膨胀 4/3，再留出 JSON 转义余量，因此取 5MB。
 */
export const MAX_XLSX_BYTES = 5 * 1024 * 1024;

export interface XlsxSheet {
  /** 工作表名；非法字符会被替换，超过 31 字符截断。 */
  name?: string;
  columns: string[];
  rows: Cell[][];
}

export interface XlsxOptions {
  /** 超过该字节数直接拒绝，避免撞 sidecar 的消息上限。 */
  maxBytes?: number;
}

/** 导出结果超出 sidecar 消息上限时抛出，由 UI 提示调小行数。 */
export class XlsxTooLargeError extends Error {
  readonly bytes: number;

  constructor(bytes: number) {
    super(`xlsx export is too large: ${bytes} bytes`);
    this.name = "XlsxTooLargeError";
    this.bytes = bytes;
  }
}

/** 单个单元格的文本上限，Excel 的硬限制。 */
const MAX_CELL_CHARS = 32767;

/** XML 1.0 不允许的控制字符（保留 \t \n \r）。 */
const INVALID_XML_CHARS = /[\u0000-\u0008\u000B\u000C\u000E-\u001F]/g;

/** 生成 .xlsx 字节（OOXML + store ZIP，零依赖）。 */
export function buildXlsx(sheet: XlsxSheet, options: XlsxOptions = {}): Uint8Array {
  const bytes = zipStore([
    { name: "[Content_Types].xml", text: contentTypesXml() },
    { name: "_rels/.rels", text: rootRelsXml() },
    { name: "xl/workbook.xml", text: workbookXml(sheet.name) },
    { name: "xl/_rels/workbook.xml.rels", text: workbookRelsXml() },
    { name: "xl/styles.xml", text: stylesXml() },
    { name: "xl/worksheets/sheet1.xml", text: worksheetXml(sheet) },
  ]);
  const limit = options.maxBytes ?? MAX_XLSX_BYTES;
  if (bytes.length > limit) throw new XlsxTooLargeError(bytes.length);
  return bytes;
}

/** 导出文件名：只用 ASCII 且不带路径，匹配 sidecar 的 downloadNamePattern。 */
export function exportFilename(at: Date = new Date()): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  const date = `${at.getFullYear()}${pad(at.getMonth() + 1)}${pad(at.getDate())}`;
  const time = `${pad(at.getHours())}${pad(at.getMinutes())}${pad(at.getSeconds())}`;
  return `query-result-${date}-${time}.xlsx`;
}

function contentTypesXml(): string {
  return (
    xmlHead() +
    '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">' +
    '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>' +
    '<Default Extension="xml" ContentType="application/xml"/>' +
    '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>' +
    '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>' +
    '<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>' +
    "</Types>"
  );
}

function rootRelsXml(): string {
  return (
    xmlHead() +
    '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' +
    '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>' +
    "</Relationships>"
  );
}

function workbookXml(rawName: string | undefined): string {
  return (
    xmlHead() +
    '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">' +
    `<sheets><sheet name="${escapeXml(sheetName(rawName))}" sheetId="1" r:id="rId1"/></sheets>` +
    "</workbook>"
  );
}

function workbookRelsXml(): string {
  return (
    xmlHead() +
    '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' +
    '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>' +
    '<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>' +
    "</Relationships>"
  );
}

/** 只有 s="0"（默认）与 s="1"（表头加粗）两种格式。 */
function stylesXml(): string {
  return (
    xmlHead() +
    '<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">' +
    '<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>' +
    '<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>' +
    '<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>' +
    '<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>' +
    '<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>' +
    '<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>' +
    '<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>' +
    "</styleSheet>"
  );
}

function worksheetXml(sheet: XlsxSheet): string {
  const columns = sheet.columns;
  const lastColumn = columnName(Math.max(0, columns.length - 1));
  const lastRow = Math.max(1, sheet.rows.length + 1);
  const dimension = `A1:${lastColumn}${lastRow}`;
  const parts: string[] = [
    xmlHead(),
    '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">',
    `<dimension ref="${dimension}"/>`,
    '<sheetViews><sheetView workbookViewId="0">' +
      '<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>' +
      "</sheetView></sheetViews>",
    "<sheetData>",
  ];

  const header = columns
    .map(
      (column, index) =>
        `<c r="${columnName(index)}1" s="1" t="inlineStr"><is>${textNode(column)}</is></c>`,
    )
    .join("");
  parts.push(`<row r="1">${header}</row>`);

  sheet.rows.forEach((row, rowIndex) => {
    const ref = rowIndex + 2;
    const cells = row
      .map((value, index) => cellXml(value, `${columnName(index)}${ref}`))
      .join("");
    parts.push(`<row r="${ref}">${cells}</row>`);
  });

  parts.push("</sheetData>");
  if (columns.length > 0) parts.push(`<autoFilter ref="${dimension}"/>`);
  parts.push("</worksheet>");
  return parts.join("");
}

/** 单元格：NULL 不写；布尔用 b 类型；数字用数值单元格，长整数与特殊值退文本。 */
function cellXml(value: Cell | undefined, ref: string): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "boolean") {
    return `<c r="${ref}" t="b"><v>${value ? 1 : 0}</v></c>`;
  }
  if (typeof value === "number") {
    const risky = !Number.isFinite(value) || (Number.isInteger(value) && Math.abs(value) >= 1e15);
    if (!risky) return `<c r="${ref}"><v>${value}</v></c>`;
    return inlineTextCell(ref, String(value));
  }
  return inlineTextCell(ref, String(value));
}

function inlineTextCell(ref: string, text: string): string {
  return `<c r="${ref}" t="inlineStr"><is>${textNode(text)}</is></c>`;
}

/**
 * 文本节点：剔除 XML 非法字符、按 Excel 上限截断、转义 & < > "；
 * 首尾有空白时加 xml:space="preserve"，避免 Excel 吃掉空格。
 */
function textNode(raw: string): string {
  const text = raw.replace(INVALID_XML_CHARS, "").slice(0, MAX_CELL_CHARS);
  const preserve = text.trim() === text ? "" : ' xml:space="preserve"';
  return `<t${preserve}>${escapeXml(text)}</t>`;
}

function escapeXml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function xmlHead(): string {
  return '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>';
}

/** Excel 的工作表名限制：31 字符，且不能含 []:*?/\。 */
function sheetName(raw: string | undefined): string {
  const name = (raw ?? "result").replace(/[[\]:*?/\\]/g, "_").trim();
  return (name || "result").slice(0, 31);
}

/** 0 → A、25 → Z、26 → AA。 */
function columnName(index: number): string {
  let name = "";
  let rest = index + 1;
  while (rest > 0) {
    const digit = (rest - 1) % 26;
    name = String.fromCharCode(65 + digit) + name;
    rest = Math.floor((rest - 1) / 26);
  }
  return name;
}

interface ZipEntry {
  name: string;
  text: string;
}

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let index = 0; index < 256; index += 1) {
    let value = index;
    for (let bit = 0; bit < 8; bit += 1) {
      value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
    }
    table[index] = value >>> 0;
  }
  return table;
})();

function crc32(data: Uint8Array): number {
  let crc = 0xffffffff;
  for (let index = 0; index < data.length; index += 1) {
    crc = CRC_TABLE[(crc ^ data[index]) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

/** 固定 1980-01-01 的 DOS 时间戳：内容不变时产物字节稳定。 */
const DOS_DATE = 33;
const DOS_TIME = 0;

/** 只做 store（不压缩）的 zip：xlsx 规范允许，省掉 DEFLATE 依赖。 */
function zipStore(entries: ZipEntry[]): Uint8Array {
  const encoder = new TextEncoder();
  const prepared = entries.map((entry) => {
    const name = encoder.encode(entry.name);
    const data = encoder.encode(entry.text);
    return { name, data, crc: crc32(data) };
  });

  let size = 22;
  for (const entry of prepared) {
    size += 30 + entry.name.length + entry.data.length + 46 + entry.name.length;
  }

  const out = new Uint8Array(size);
  const view = new DataView(out.buffer);
  let at = 0;
  const writeU16 = (value: number) => {
    view.setUint16(at, value, true);
    at += 2;
  };
  const writeU32 = (value: number) => {
    view.setUint32(at, value, true);
    at += 4;
  };

  const offsets: number[] = [];
  for (const entry of prepared) {
    offsets.push(at);
    writeU32(0x04034b50);
    writeU16(20);
    writeU16(0);
    writeU16(0);
    writeU16(DOS_TIME);
    writeU16(DOS_DATE);
    writeU32(entry.crc);
    writeU32(entry.data.length);
    writeU32(entry.data.length);
    writeU16(entry.name.length);
    writeU16(0);
    out.set(entry.name, at);
    at += entry.name.length;
    out.set(entry.data, at);
    at += entry.data.length;
  }

  const centralStart = at;
  prepared.forEach((entry, index) => {
    writeU32(0x02014b50);
    writeU16(20);
    writeU16(20);
    writeU16(0);
    writeU16(0);
    writeU16(DOS_TIME);
    writeU16(DOS_DATE);
    writeU32(entry.crc);
    writeU32(entry.data.length);
    writeU32(entry.data.length);
    writeU16(entry.name.length);
    writeU16(0);
    writeU16(0);
    writeU16(0);
    writeU16(0);
    writeU32(0);
    writeU32(offsets[index]);
    out.set(entry.name, at);
    at += entry.name.length;
  });

  // 中央目录大小必须在写 EOCD 之前取：writeU32/writeU16 自己也会推进 at。
  const centralSize = at - centralStart;
  writeU32(0x06054b50);
  writeU16(0);
  writeU16(0);
  writeU16(prepared.length);
  writeU16(prepared.length);
  writeU32(centralSize);
  writeU32(centralStart);
  writeU16(0);

  return out;
}
