// Build config for the chat app. Three decisions worth reading before changing anything.
//
// 1. The output goes to ../dist and is COMMITTED. The Go binary embeds it
//    (`//go:embed all:dist`), so `go build` and `go install` keep working on a machine
//    with no Node — the guide's install instructions do not grow an npm step, and CI's
//    Pages job stays Go-only. `TestBuiltUIMatchesItsSources` fails when dist is stale.
// 2. Assets are self-hosted and content-hashed. There is no CDN and no `integrity`
//    attribute any more: the bundle is same-origin, so SRI would be pinning bytes to
//    themselves. web/vendor.sha384 still exists — it pins the *docs pages*, which are
//    static files on GitHub Pages with no build step at all.
// 3. Mermaid stays out of the entry chunk. It is ~3.4 MB and only an answer that
//    contains a diagram needs it, so src/lib/diagram.js imports it dynamically and
//    Rollup gives it its own chunk. Nothing else may import it statically — that would
//    silently move it into the first paint.
import { fileURLToPath } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import { stamp } from './scripts/stamp.js'

export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  plugins: [
    // <nes-*> are custom elements from the design system, not Vue components. This is a
    // *compile-time* decision: SFC templates are compiled by this plugin, so telling the
    // runtime alone would be too late and every <nes-icon> would warn about a missing
    // component and render nothing.
    vue({ template: { compilerOptions: { isCustomElement: tag => tag.startsWith('nes-') } } }),
    stamp(),
  ],
  build: {
    outDir: fileURLToPath(new URL('../dist', import.meta.url)),
    emptyOutDir: true,
    // The binary serves /assets/* with a one-year immutable cache, which is only safe
    // because every name carries a content hash.
    assetsDir: 'assets',
    // No source maps in the committed build. They were 12 MB of the 16 — generated
    // files, rewritten wholesale on every build, in a repository that has to stay
    // clonable. Debugging happens in `npm run dev`, which has the real sources.
    sourcemap: false,
    // Fail loudly rather than silently shipping a 3 MB entry chunk.
    chunkSizeWarningLimit: 700,
  },
  server: {
    // `npm run dev` serves the UI with HMR and forwards the API to the Go server, so
    // the two run side by side: edit a .vue file, see it without a rebuild, and still
    // talk to the real engine. Start the backend with `make server` first.
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
})
