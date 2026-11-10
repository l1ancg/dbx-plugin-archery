import { readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), "utf8");

const version = JSON.parse(read("package.json")).version;
const manifest = JSON.parse(read("manifest.json"));
const mainGo = read("backend/main.go");

const problems = [];
if (manifest.version !== version) {
  problems.push(`manifest.json version=${manifest.version} != package.json ${version}`);
}
if (!mainGo.includes(`Version:      "${version}"`)) {
  problems.push(`backend/main.go Version is not ${version}`);
}
if (!mainGo.includes(`ID:           "${manifest.id}"`)) {
  problems.push(`backend/main.go plugin ID is not ${manifest.id}`);
}

if (problems.length > 0) {
  console.error(problems.join("\n"));
  process.exit(1);
}
console.log(`version check ok: ${version}`);
