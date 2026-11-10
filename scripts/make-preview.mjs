import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const htmlPath = resolve(root, "ui/index.html");
const bridgePath = resolve(root, "preview/mock.js");
const outPath = resolve(root, "preview/index.html");

if (!existsSync(htmlPath)) {
  console.error("ui/index.html 不存在，请先执行 pnpm build");
  process.exit(1);
}

const html = readFileSync(htmlPath, "utf8");
const bridge = readFileSync(bridgePath, "utf8");
const injected = html.replace(
  "<head>",
  `<head>\n<script>\n${bridge}\n</script>`,
);

mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, injected, "utf8");
console.log(`preview ready: ${outPath}`);
