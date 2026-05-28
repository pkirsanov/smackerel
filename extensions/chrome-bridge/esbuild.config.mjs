// esbuild build pipeline for the smackerel chrome-bridge extension (spec 058 §8.1).
// Produces dist/extension/chrome-bridge/ containing manifest, background SW bundle,
// options-page bundle + HTML, and icons. Byte-reproducible packaging is the
// responsibility of scripts/commands/build-chrome-bridge.sh (Scope 4).

import { build } from "esbuild";
import { mkdir, copyFile, rm, readdir, stat } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const outDir = join(__dirname, "dist", "extension", "chrome-bridge");

const production = process.env.NODE_ENV !== "development";

async function copyTree(src, dst) {
  const s = await stat(src);
  if (s.isDirectory()) {
    await mkdir(dst, { recursive: true });
    for (const entry of await readdir(src)) {
      await copyTree(join(src, entry), join(dst, entry));
    }
  } else {
    await mkdir(dirname(dst), { recursive: true });
    await copyFile(src, dst);
  }
}

async function run() {
  if (existsSync(outDir)) {
    await rm(outDir, { recursive: true, force: true });
  }
  await mkdir(join(outDir, "options"), { recursive: true });

  const common = {
    bundle: true,
    format: "esm",
    target: "es2022",
    platform: "browser",
    minify: production,
    sourcemap: production ? false : "inline",
    legalComments: "none",
  };

  await build({
    ...common,
    entryPoints: [join(__dirname, "src", "background", "index.ts")],
    outfile: join(outDir, "background.js"),
  });

  await build({
    ...common,
    entryPoints: [join(__dirname, "src", "options", "index.ts")],
    outfile: join(outDir, "options", "index.js"),
  });

  await copyFile(
    join(__dirname, "manifest.json"),
    join(outDir, "manifest.json"),
  );
  await copyFile(
    join(__dirname, "src", "options", "index.html"),
    join(outDir, "options", "index.html"),
  );

  const iconsDir = join(__dirname, "icons");
  if (existsSync(iconsDir)) {
    await copyTree(iconsDir, join(outDir, "icons"));
  }
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
