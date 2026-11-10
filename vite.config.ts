import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import { viteSingleFile } from "vite-plugin-singlefile";

export default defineConfig({
  base: "./",
  plugins: [vue(), viteSingleFile()],
  build: {
    outDir: "ui",
    emptyOutDir: true,
    target: "es2022",
    assetsInlineLimit: 1024 * 1024,
    chunkSizeWarningLimit: 4096,
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
    fileParallelism: false,
  },
});
