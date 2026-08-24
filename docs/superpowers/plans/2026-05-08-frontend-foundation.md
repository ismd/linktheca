# Frontend Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the `web/` SPA project (Vite + React 19 + TS strict + Tailwind v4 + shadcn/ui), a responsive AppShell, an API client skeleton, an auth store skeleton, the tooling, and the deploy configs. Functionally this is a shell you can navigate between route stubs in, but with no auth and no Library — those come in the following plans.

**Architecture:** The frontend lives in the monorepo's `web/` subdirectory. The Vite dev server proxies `/api/*` to the backend on `:8080`; in production that is nginx. Layout: a single `<Sidebar />` renders either pinned (`lg+`) or inside a Radix `<Dialog>` overlay drawer (`< lg`). API access goes through one `apiFetch` wrapper, and auth state lives in Zustand.

**Tech Stack:** Vite 5, React 19, TypeScript strict, Tailwind v4, shadcn/ui, React Router v7 (data mode), TanStack Query v5, Zustand, Radix primitives, lucide-react, Vitest + React Testing Library + MSW.

**Spec:** `docs/superpowers/specs/2026-05-08-frontend-foundation-library-design.md`.

---

## File structure (created by this plan)

```
web/
  package.json
  package-lock.json
  tsconfig.json
  tsconfig.node.json
  vite.config.ts
  index.html
  components.json              # shadcn config
  eslint.config.js
  .prettierrc
  vitest.config.ts
  Dockerfile
  nginx.conf
  src/
    main.tsx
    App.tsx
    routes/
      __root.tsx
      _public.tsx
      index.tsx
      login.tsx
      register.tsx
      library._index.tsx
      library.$id.tsx
      settings.tsx
      not-found.tsx
    features/
      auth/
        store.ts
        store.test.ts
    shared/
      api/
        client.ts
        client.test.ts
        errors.ts
        errors.test.ts
      ui/                       # shadcn copies (button, card, dialog, input, label, alert-dialog)
      layout/
        AppShell.tsx
        AppShell.test.tsx
        Sidebar.tsx
        Sidebar.test.tsx
        Topbar.tsx
        MobileDrawer.tsx
        PaperGrainOverlay.tsx
        PageHeader.tsx
      lib/
        cn.ts
      hooks/                    # empty for now; it appears in the Library plan
    styles/
      globals.css
    test/
      setup.ts

compose.prod.yaml                # at the repo root, for the prod web+backend deploy
.github/workflows/ci.yml         # modified: a frontend job is added
```

---

## Group 1 — Project skeleton

### Task 1: Initialize Vite + React 19 + TypeScript strict in `web/`

**Files:**
- Create: `web/package.json`
- Create: `web/tsconfig.json`
- Create: `web/tsconfig.app.json`
- Create: `web/tsconfig.node.json`
- Create: `web/vite.config.ts`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/.gitignore`

- [x] **Step 1: Create `web/package.json`**

```json
{
  "name": "linktheca-web",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.3.0",
    "typescript": "^5.6.0",
    "vite": "^5.4.0"
  }
}
```

- [x] **Step 2: Create `web/tsconfig.json`** (root, references + alias)

The canonical create-vite split template: the root tsconfig is an empty container with `references`, and the concrete settings live in `tsconfig.app.json` and `tsconfig.node.json`. That separates the `src/` (DOM) and `vite.config.ts` (Node) contexts, which need different `lib`/`module`.

We also duplicate `paths` in the root config. The reason: the shadcn CLI (`npx shadcn add ...`) resolves the alias from `components.json` through the **root** `tsconfig.json` and does not follow `references`. Without that block the CLI cannot find `@/*`, treats `@/shared/ui/...` as a literal path, and creates a `web/@/` directory instead of files in `web/src/shared/ui/`.

`baseUrl` is deliberately omitted: TypeScript 5.5+ resolves `paths` relative to the `tsconfig.json` itself without it, and in TypeScript 6 the option is deprecated (TS5101) and will be removed in TS 7. Current shadcn CLI versions work with `paths` and no `baseUrl`.

```json
{
  "files": [],
  "references": [
    { "path": "./tsconfig.app.json" },
    { "path": "./tsconfig.node.json" }
  ],
  "compilerOptions": {
    "paths": {
      "@/*": ["./src/*"]
    }
  }
}
```

- [x] **Step 3: Create `web/tsconfig.app.json`** (for `src/`)

The key differences from the standard template:
- `types: ["vite/client"]` — the types for `import.meta.env`, `import.meta.hot`, and asset imports (`?url`, `?raw`, `?worker`).
- `verbatimModuleSyntax: true` — forces explicit `import type`; it helps Vite 8's Oxc transpiler and improves tree-shaking.
- `isolatedModules: true` — mandatory for Vite (Oxc does not read types while transpiling).
- `composite: true` + `tsBuildInfoFile` — needed for project references so `tsc -b` does not complain with TS6306.
- `paths: { "@/*": ["./src/*"] }` — the alias, kept in sync with `vite.config.ts`.

```json
{
  "compilerOptions": {
    "composite": true,
    "tsBuildInfoFile": "./node_modules/.tmp/tsconfig.app.tsbuildinfo",
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "types": ["vite/client"],

    "moduleResolution": "Bundler",
    "allowImportingTsExtensions": true,
    "verbatimModuleSyntax": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "jsx": "react-jsx",
    "resolveJsonModule": true,
    "esModuleInterop": true,

    "noEmit": true,
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedIndexedAccess": true,

    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src"]
}
```

- [x] **Step 3b: Create `web/tsconfig.node.json`** (for `vite.config.ts` and `vitest.config.ts`)

```json
{
  "compilerOptions": {
    "composite": true,
    "tsBuildInfoFile": "./node_modules/.tmp/tsconfig.node.tsbuildinfo",
    "target": "ES2023",
    "lib": ["ES2023"],
    "module": "ESNext",
    "skipLibCheck": true,

    "moduleResolution": "Bundler",
    "allowImportingTsExtensions": true,
    "verbatimModuleSyntax": true,
    "isolatedModules": true,
    "moduleDetection": "force",

    "noEmit": true,
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedIndexedAccess": true
  },
  "include": ["vite.config.ts"]
}
```

> **Note:** `vitest.config.ts` arrives in Task 15 — it has to be added to `include` then.

- [x] **Step 4: Create `web/vite.config.ts`**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
});
```

- [x] **Step 5: Create `web/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Linktheca</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [x] **Step 6: Create `web/src/main.tsx`**

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [x] **Step 7: Create `web/src/App.tsx`**

```tsx
export default function App() {
  return <h1>Linktheca</h1>;
}
```

- [x] **Step 8: Create `web/.gitignore`**

```
node_modules
dist
.env.local
*.log
```

- [x] **Step 9: Install the dependencies**

Run: `cd web && npm install`
Expected: `package-lock.json` is created, `node_modules/` appears, no errors.

- [x] **Step 10: Verify dev server**

Run: `cd web && npm run dev`
Expected: vite comes up on `:5173`, and the browser shows "Linktheca" in large text. Stop it with Ctrl+C.

- [x] **Step 11: Verify build**

Run: `cd web && npm run build`
Expected: `dist/` is created, containing `index.html` and `assets/`.

- [x] **Step 12: Commit**

```bash
git add web/.gitignore web/package.json web/package-lock.json web/tsconfig.json web/tsconfig.app.json web/tsconfig.node.json web/vite.config.ts web/index.html web/src/main.tsx web/src/App.tsx
git commit -m "feat(web): initialize Vite + React 19 + TypeScript strict skeleton"
```

---

### Task 2: Tailwind CSS v4 + design tokens

**Files:**
- Modify: `web/package.json` (deps)
- Modify: `web/vite.config.ts` (add tailwind plugin)
- Create: `web/src/styles/globals.css`
- Modify: `web/src/main.tsx` (import globals)
- Modify: `web/src/App.tsx` (verify token usage)

- [x] **Step 1: Install Tailwind v4**

Run: `cd web && npm install -D tailwindcss@^4 @tailwindcss/vite@^4`
Expected: the packages are added to devDependencies.

- [x] **Step 2: Wire the tailwind plugin into `web/vite.config.ts`**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
});
```

- [x] **Step 3: Create `web/src/styles/globals.css`**

```css
@import "tailwindcss";

@theme {
  --color-paper: #f3ece0;
  --color-paper-2: #ebe3d2;
  --color-paper-3: #e2d9c3;
  --color-ink: #1a1814;
  --color-ink-2: #2d2a24;
  --color-ink-3: #4a4438;
  --color-muted: #8a8275;
  --color-muted-2: #a69d8a;
  --color-rule: #d4cdbe;
  --color-rule-2: #e2dbc9;
  --color-vermillion: #c83832;
  --color-vermillion-dark: #9c241e;
  --color-ochre: #c89632;
  --color-sage: #6e8458;
  --color-plum: #6b3a4e;

  --font-display: "Cormorant Garamond", ui-serif, Georgia, serif;
  --font-body: "Newsreader", Georgia, serif;
  --font-mono: "IBM Plex Mono", ui-monospace, monospace;
}

html {
  scroll-behavior: smooth;
}

body {
  font-family: var(--font-body);
  background: var(--color-paper);
  color: var(--color-ink);
  font-feature-settings: "ss01", "ss02", "kern";
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}

::selection {
  background: var(--color-vermillion);
  color: var(--color-paper);
}

::-webkit-scrollbar {
  width: 12px;
  height: 12px;
}
::-webkit-scrollbar-track {
  background: var(--color-paper-2);
}
::-webkit-scrollbar-thumb {
  background: var(--color-muted-2);
  border: 3px solid var(--color-paper-2);
  border-radius: 0;
}
::-webkit-scrollbar-thumb:hover {
  background: var(--color-muted);
}

button:focus-visible,
a:focus-visible,
input:focus-visible,
textarea:focus-visible {
  outline: 2px solid var(--color-vermillion);
  outline-offset: 2px;
}
input:focus,
textarea:focus,
select:focus {
  outline: none;
}
```

- [x] **Step 4: Import globals in `web/src/main.tsx`**

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles/globals.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [x] **Step 5: Use the tokens in `web/src/App.tsx` as a check**

```tsx
export default function App() {
  return (
    <div className="min-h-screen bg-paper text-ink p-8">
      <h1 className="font-display text-5xl text-vermillion">Linktheca</h1>
    </div>
  );
}
```

- [x] **Step 6: Verify**

Run: `cd web && npm run dev`
Expected: the page sits on a warm beige (paper) background with a vermillion heading. Check it in the browser, then Ctrl+C.

- [x] **Step 7: Commit**

```bash
git add web/package.json web/package-lock.json web/vite.config.ts web/src/styles/globals.css web/src/main.tsx web/src/App.tsx
git commit -m "feat(web): add Tailwind v4 with editorial design tokens"
```

---

### Task 3: Fonts through @fontsource

**Files:**
- Modify: `web/package.json` (deps)
- Modify: `web/src/main.tsx` (font imports)

- [x] **Step 1: Install the fonts**

Run:
```bash
cd web && npm install \
  @fontsource/cormorant-garamond \
  @fontsource-variable/newsreader \
  @fontsource/ibm-plex-mono
```

Expected: three packages are added to dependencies.

- [x] **Step 2: Import them in `web/src/main.tsx`**

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

import "@fontsource/cormorant-garamond/400.css";
import "@fontsource/cormorant-garamond/500.css";
import "@fontsource/cormorant-garamond/600.css";
import "@fontsource/cormorant-garamond/700.css";
import "@fontsource/cormorant-garamond/400-italic.css";
import "@fontsource-variable/newsreader";
import "@fontsource/ibm-plex-mono/400.css";
import "@fontsource/ibm-plex-mono/500.css";
import "@fontsource/ibm-plex-mono/600.css";

import "./styles/globals.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [x] **Step 3: Verify**

Run: `cd web && npm run dev` and check in the browser that the "Linktheca" heading is in Cormorant Garamond, not the default serif. The Network tab shows the `.woff2` files loading from @fontsource. Ctrl+C.

- [x] **Step 4: Commit**

```bash
git add web/package.json web/package-lock.json web/src/main.tsx
git commit -m "feat(web): bundle Cormorant Garamond, Newsreader, IBM Plex Mono via @fontsource"
```

---

### Task 4: Move the prototype's utility classes into `globals.css`

**Files:**
- Modify: `web/src/styles/globals.css`

- [x] **Step 1: Add an `@layer components` block at the end of `globals.css`**

```css
@layer components {
  /* Paper grain overlay (used as fixed inset-0 div) */
  .grain-overlay {
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 1;
    opacity: 0.5;
    mix-blend-mode: multiply;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='240' height='240'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 0.1 0 0 0 0 0.08 0 0 0 0 0.05 0 0 0 0.08 0'/%3E%3C/filter%3E%3Crect width='240' height='240' filter='url(%23n)'/%3E%3C/svg%3E");
  }

  .paper-surface {
    background-color: var(--color-paper);
    background-image:
      radial-gradient(ellipse at 30% 20%, rgba(200, 56, 50, 0.015) 0%, transparent 50%),
      radial-gradient(ellipse at 70% 80%, rgba(138, 130, 117, 0.04) 0%, transparent 60%);
  }

  /* Rules */
  .rule-dotted {
    background-image: linear-gradient(to right, var(--color-muted) 40%, transparent 40%);
    background-size: 5px 1px;
    background-repeat: repeat-x;
    background-position: 0 50%;
    height: 1px;
  }
  .rule-double {
    border-top: 1px solid var(--color-ink);
    border-bottom: 1px solid var(--color-ink);
    height: 5px;
  }
  .rule-thick {
    border-top: 2px solid var(--color-ink);
  }

  /* Small caps labels */
  .label-sc {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    font-weight: 500;
  }
  .label-sc-lg {
    font-family: var(--font-mono);
    font-size: 0.75rem;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    font-weight: 500;
  }

  /* Display heading tuning */
  .display-tight {
    font-family: var(--font-display);
    letter-spacing: -0.02em;
    line-height: 0.95;
    font-weight: 500;
  }

  /* Stamp — library-style */
  .stamp {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.55rem;
    border: 1.5px solid currentColor;
    font-family: var(--font-mono);
    font-size: 0.6rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-weight: 500;
    transform: rotate(-0.8deg);
    white-space: nowrap;
  }
  .stamp-flat {
    transform: none;
  }

  /* Sidebar nav item */
  .nav-item {
    position: relative;
    transition: all 0.2s ease;
  }
  .nav-item::after {
    content: "";
    position: absolute;
    left: 0;
    top: 50%;
    transform: translateY(-50%) scaleX(0);
    transform-origin: left;
    width: 12px;
    height: 2px;
    background: var(--color-vermillion);
    transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  }
  .nav-item.active::after {
    transform: translateY(-50%) scaleX(1);
  }
  .nav-item:hover:not(.active)::after {
    transform: translateY(-50%) scaleX(0.5);
    background: var(--color-muted);
  }

  /* Tag pill */
  .tag-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.15rem 0.55rem 0.2rem;
    background: var(--color-paper-2);
    border: 1px solid var(--color-rule);
    font-family: var(--font-mono);
    font-size: 0.65rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-ink-3);
    transition: all 0.2s ease;
    cursor: pointer;
  }
  .tag-pill:hover {
    background: var(--color-ink);
    color: var(--color-paper);
    border-color: var(--color-ink);
  }

  /* Icon button */
  .icon-btn {
    width: 38px;
    height: 38px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--color-rule);
    color: var(--color-ink-3);
    transition: all 0.2s ease;
    cursor: pointer;
    background: transparent;
    flex-shrink: 0;
  }
  .icon-btn:hover {
    background: var(--color-paper-2);
    color: var(--color-ink);
    border-color: var(--color-ink-3);
  }

  /* Skeleton loading */
  .skeleton {
    background: linear-gradient(
      90deg,
      rgba(212, 205, 190, 0.3) 0%,
      rgba(212, 205, 190, 0.6) 50%,
      rgba(212, 205, 190, 0.3) 100%
    );
    background-size: 200% 100%;
    animation: skeleton-wave 1.4s ease-in-out infinite;
  }

  /* Modal backdrop */
  .modal-backdrop {
    background-color: rgba(26, 24, 20, 0.55);
    backdrop-filter: blur(3px);
    -webkit-backdrop-filter: blur(3px);
  }

  /* Drop cap (used in reader) */
  .drop-cap > p:first-of-type::first-letter {
    font-family: var(--font-display);
    float: left;
    font-size: 5.25rem;
    line-height: 0.8;
    padding: 0.35rem 0.75rem 0 0;
    font-weight: 700;
    color: var(--color-vermillion);
  }

  /* Reader prose */
  .prose-reader {
    font-family: var(--font-body);
    font-size: 1.2rem;
    line-height: 1.7;
    color: var(--color-ink-2);
  }
  .prose-reader p {
    margin-bottom: 1.3em;
  }
  .prose-reader p:first-of-type {
    margin-top: 0;
  }
  .prose-reader h2 {
    font-family: var(--font-display);
    font-size: 1.8rem;
    font-weight: 600;
    margin-top: 2.5em;
    margin-bottom: 0.8em;
    letter-spacing: -0.015em;
    color: var(--color-ink);
  }
  .prose-reader blockquote {
    border-left: 3px solid var(--color-vermillion);
    padding: 0.3em 0 0.3em 1.5em;
    margin: 2em 0;
    font-style: italic;
    font-size: 1.35rem;
    color: var(--color-ink);
    font-family: var(--font-display);
    line-height: 1.45;
  }
  .prose-reader code {
    font-family: var(--font-mono);
    font-size: 0.88em;
    background: var(--color-paper-2);
    padding: 0.1em 0.35em;
    border: 1px solid var(--color-rule);
  }
  .prose-reader a {
    color: var(--color-vermillion);
    text-decoration: underline;
    text-decoration-style: dotted;
    text-underline-offset: 3px;
  }
  .prose-reader strong {
    font-weight: 700;
    color: var(--color-ink);
  }

  /* Mock image gradient backgrounds for cards/heroes */
  .img-1 {
    background:
      radial-gradient(circle at 70% 20%, rgba(200, 56, 50, 0.7), transparent 45%),
      radial-gradient(circle at 30% 70%, rgba(107, 58, 78, 0.9), transparent 50%),
      linear-gradient(135deg, #2d2a24, #1a1814);
  }
  .img-2 {
    background:
      radial-gradient(circle at 20% 80%, rgba(200, 150, 50, 0.6), transparent 50%),
      linear-gradient(120deg, #3d3a2e 0%, #1a1814 70%);
  }
  .img-3 {
    background:
      radial-gradient(ellipse at 50% 30%, rgba(110, 132, 88, 0.7), transparent 60%),
      linear-gradient(180deg, #1a2418, #0f1a0f);
  }
  .img-4 {
    background:
      radial-gradient(circle at 80% 70%, rgba(200, 56, 50, 0.55), transparent 50%),
      linear-gradient(45deg, #1a1814, #2d2a24);
  }
  .img-5 {
    background:
      repeating-linear-gradient(45deg, rgba(200, 150, 50, 0.08) 0 2px, transparent 2px 12px),
      linear-gradient(120deg, #2d2a24, #1a1814);
  }
  .img-6 {
    background:
      radial-gradient(circle at 30% 30%, rgba(200, 56, 50, 0.6), transparent 45%),
      radial-gradient(circle at 70% 70%, rgba(200, 150, 50, 0.4), transparent 50%),
      #1a1814;
  }
  .img-7 {
    background: conic-gradient(from 120deg at 40% 60%, #6b3a4e, #1a1814, #2d2a24, #6b3a4e);
  }
  .img-8 {
    background:
      radial-gradient(ellipse at 60% 40%, rgba(110, 132, 88, 0.5), transparent 55%),
      linear-gradient(200deg, #1a1814, #3a4a2e 120%);
  }
  .img-9 {
    background:
      linear-gradient(135deg, rgba(200, 56, 50, 0.25), transparent 60%),
      repeating-linear-gradient(0deg, rgba(255, 255, 255, 0.03) 0 1px, transparent 1px 8px),
      #1a1814;
  }
  .img-10 {
    background:
      radial-gradient(circle at 50% 50%, rgba(200, 150, 50, 0.5), transparent 60%),
      #241e18;
  }
}

@keyframes skeleton-wave {
  0% {
    background-position: -200% 0;
  }
  100% {
    background-position: 200% 0;
  }
}

@keyframes fade-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
.animate-fade-in {
  animation: fade-in 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
}
```

- [x] **Step 2: Verify in the browser that the classes work (a smoke check)**

Temporarily add to `App.tsx`:

```tsx
export default function App() {
  return (
    <div className="paper-surface min-h-screen p-8">
      <h1 className="display-tight text-5xl text-vermillion">Linktheca</h1>
      <p className="label-sc mt-2">A private archive for the careful reader</p>
      <div className="rule-dotted my-6" />
      <span className="stamp text-vermillion">Read</span>
    </div>
  );
}
```

Run: `cd web && npm run dev` — the browser shows the paper-surface background, a dotted rule, and a tilted stamp reading "READ" in the mono font. Ctrl+C.

- [x] **Step 3: Revert the temporary changes in App.tsx**

```tsx
export default function App() {
  return (
    <div className="min-h-screen bg-paper text-ink p-8">
      <h1 className="font-display text-5xl text-vermillion">Linktheca</h1>
    </div>
  );
}
```

- [x] **Step 4: Commit**

```bash
git add web/src/styles/globals.css
git commit -m "feat(web): port editorial utility classes from prototype"
```

---

### Task 5: The Vite dev proxy for `/api/*`

**Files:**
- Modify: `web/vite.config.ts`

- [x] **Step 1: Add the proxy to `web/vite.config.ts`**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ""),
      },
    },
  },
});
```

- [x] **Step 2: Verify proxy works**

Bring up the backend (in a separate terminal): `make dev-db && make run`
Run in the first terminal: `cd web && npm run dev`
Open `http://localhost:5173/api/healthz` in the browser
Expected: the response `ok` (the backend's `/healthz` returns `ok`).

Stop both processes.

- [x] **Step 3: Commit**

```bash
git add web/vite.config.ts
git commit -m "feat(web): proxy /api/* to backend during dev"
```

---

## Group 2 — shadcn + Routing

### Task 6: Install shadcn/ui and the base primitives

**Files:**
- Create: `web/components.json`
- Create: `web/src/shared/lib/cn.ts`
- Create: `web/src/shared/ui/button.tsx`
- Create: `web/src/shared/ui/card.tsx`
- Create: `web/src/shared/ui/dialog.tsx`
- Create: `web/src/shared/ui/input.tsx`
- Create: `web/src/shared/ui/label.tsx`
- Create: `web/src/shared/ui/alert-dialog.tsx`
- Modify: `web/package.json` (the shadcn CLI adds the deps)

- [x] **Step 1: Install the base dependencies by hand**

Run:
```bash
cd web && npm install \
  clsx tailwind-merge class-variance-authority \
  @radix-ui/react-dialog @radix-ui/react-alert-dialog \
  @radix-ui/react-label @radix-ui/react-slot \
  lucide-react
```

- [x] **Step 2: Create `web/src/shared/lib/cn.ts`**

```ts
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

- [x] **Step 3: Create `web/components.json`**

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "new-york",
  "rsc": false,
  "tsx": true,
  "tailwind": {
    "config": "",
    "css": "src/styles/globals.css",
    "baseColor": "neutral",
    "cssVariables": true
  },
  "aliases": {
    "components": "@/shared/ui",
    "utils": "@/shared/lib/cn",
    "ui": "@/shared/ui",
    "lib": "@/shared/lib",
    "hooks": "@/shared/hooks"
  },
  "iconLibrary": "lucide"
}
```

- [x] **Step 4: Install the shadcn primitives through the CLI**

Run:
```bash
cd web && npx shadcn@latest add button card dialog input label alert-dialog --yes
```

Expected: files appear in `src/shared/ui/`. The CLI may ask questions — answer with the defaults.

- [x] **Step 5: Verify the import works**

Temporarily change `web/src/App.tsx`:

```tsx
import { Button } from "@/shared/ui/button";

export default function App() {
  return (
    <div className="min-h-screen bg-paper text-ink p-8">
      <h1 className="font-display text-5xl text-vermillion">Linktheca</h1>
      <Button className="mt-4">Test button</Button>
    </div>
  );
}
```

Run: `cd web && npm run dev` — the button renders. Ctrl+C.

- [x] **Step 6: Revert App.tsx**

```tsx
export default function App() {
  return (
    <div className="min-h-screen bg-paper text-ink p-8">
      <h1 className="font-display text-5xl text-vermillion">Linktheca</h1>
    </div>
  );
}
```

- [x] **Step 7: Commit**

```bash
git add web/package.json web/package-lock.json web/components.json web/src/shared/lib/cn.ts web/src/shared/ui/
git commit -m "feat(web): set up shadcn/ui with button, card, dialog, input, label, alert-dialog"
```

---

### Task 7: Install React Router v7 and assemble the route tree

**Files:**
- Modify: `web/package.json` (deps)
- Create: `web/src/routes/__root.tsx`
- Create: `web/src/routes/__app.tsx`
- Create: `web/src/routes/_public.tsx`
- Create: `web/src/routes/index.tsx`
- Create: `web/src/routes/login.tsx`
- Create: `web/src/routes/register.tsx`
- Create: `web/src/routes/library._index.tsx`
- Create: `web/src/routes/library.$id.tsx`
- Create: `web/src/routes/settings.tsx`
- Create: `web/src/routes/not-found.tsx`
- Modify: `web/src/App.tsx`

- [x] **Step 1: Install React Router**

Run: `cd web && npm install react-router@^7`

> In v7 `react-router-dom` is merged into the single `react-router` package. Every hook and component is imported from `react-router`; the one exception is the DOM-specific `RouterProvider`, which comes from the `react-router/dom` deep path.

- [x] **Step 2: Create the `web/src/routes/__root.tsx` stub**

```tsx
import { Outlet } from "react-router";

export default function RootLayout() {
  return (
    <div className="min-h-screen bg-paper text-ink">
      <Outlet />
    </div>
  );
}
```

- [x] **Step 3: Create `web/src/routes/_public.tsx`**

```tsx
import { Outlet } from "react-router";

export default function PublicLayout() {
  return (
    <div className="paper-surface min-h-screen flex items-center justify-center p-8">
      <div className="w-full max-w-md">
        <Outlet />
      </div>
    </div>
  );
}
```

- [x] **Step 3b: Create `web/src/routes/__app.tsx` (a stub until Task 12)**

```tsx
import { Outlet } from "react-router";

export default function AppLayout() {
  return <Outlet />;
}
```

- [x] **Step 4: Create stubs for every page**

`web/src/routes/index.tsx`:

```tsx
import { Navigate } from "react-router";

export default function IndexRoute() {
  return <Navigate to="/library" replace />;
}
```

`web/src/routes/login.tsx`:

```tsx
export default function LoginRoute() {
  return (
    <div>
      <h1 className="font-display text-4xl text-ink">Sign in</h1>
      <p className="label-sc mt-2 text-muted">Login form goes here.</p>
    </div>
  );
}
```

`web/src/routes/register.tsx`:

```tsx
export default function RegisterRoute() {
  return (
    <div>
      <h1 className="font-display text-4xl text-ink">Create account</h1>
      <p className="label-sc mt-2 text-muted">Register form goes here.</p>
    </div>
  );
}
```

`web/src/routes/library._index.tsx`:

```tsx
export default function LibraryListRoute() {
  return (
    <div className="p-8">
      <h1 className="font-display text-5xl text-ink">Library</h1>
      <p className="label-sc mt-2 text-muted">List of saved articles.</p>
    </div>
  );
}
```

`web/src/routes/library.$id.tsx`:

```tsx
import { useParams } from "react-router";

export default function LibraryItemRoute() {
  const { id } = useParams();
  return (
    <div className="p-8">
      <h1 className="font-display text-3xl text-ink">Reader: {id}</h1>
    </div>
  );
}
```

`web/src/routes/settings.tsx`:

```tsx
export default function SettingsRoute() {
  return (
    <div className="p-8">
      <h1 className="font-display text-5xl text-ink">Settings</h1>
      <p className="label-sc mt-2 text-muted">Coming soon.</p>
    </div>
  );
}
```

`web/src/routes/not-found.tsx`:

```tsx
import { Link } from "react-router";

export default function NotFoundRoute() {
  return (
    <div className="paper-surface min-h-screen flex flex-col items-center justify-center">
      <p className="font-display text-vermillion text-9xl leading-none">404</p>
      <p className="label-sc mt-4">Page not found</p>
      <Link to="/" className="mt-6 underline decoration-dotted text-ink">
        Back to library
      </Link>
    </div>
  );
}
```

- [x] **Step 5: Assemble the router and wire it into `web/src/App.tsx`**

```tsx
import { createBrowserRouter } from "react-router";
import { RouterProvider } from "react-router/dom";
import RootLayout from "./routes/__root";
import AppLayout from "./routes/__app";
import PublicLayout from "./routes/_public";
import IndexRoute from "./routes/index";
import LoginRoute from "./routes/login";
import RegisterRoute from "./routes/register";
import LibraryListRoute from "./routes/library._index";
import LibraryItemRoute from "./routes/library.$id";
import SettingsRoute from "./routes/settings";
import NotFoundRoute from "./routes/not-found";

const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { index: true, element: <IndexRoute /> },
      {
        element: <PublicLayout />,
        children: [
          { path: "login", element: <LoginRoute /> },
          { path: "register", element: <RegisterRoute /> },
        ],
      },
      {
        element: <AppLayout />,
        children: [
          { path: "library", element: <LibraryListRoute /> },
          { path: "library/:id", element: <LibraryItemRoute /> },
          { path: "settings", element: <SettingsRoute /> },
        ],
      },
      { path: "*", element: <NotFoundRoute /> },
    ],
  },
]);

export default function App() {
  return <RouterProvider router={router} />;
}
```

- [x] **Step 6: Verify navigation**

Run: `cd web && npm run dev`. In the browser, check:
- `/` → redirects to `/library` ("Library" is visible).
- `/login`, `/register` → the public layout, a centred card.
- `/library/42` → «Reader: 42».
- `/settings` → «Settings».
- `/zzz` → the 404 page.

Ctrl+C.

- [x] **Step 7: Commit**

```bash
git add web/package.json web/package-lock.json web/src/routes/ web/src/App.tsx
git commit -m "feat(web): wire React Router v7 with route stubs and 404 page"
```

**Note:** `__app.tsx` is just `<Outlet />` for now. In Task 12 it starts wrapping the content in `<AppShell>`. That keeps the route structure clean, with no conditional logic in `__root.tsx`.

---

## Group 3 — Layout components

### Task 8: PaperGrainOverlay component

**Files:**
- Create: `web/src/shared/layout/PaperGrainOverlay.tsx`
- Modify: `web/src/routes/__root.tsx`

- [x] **Step 1: Create `web/src/shared/layout/PaperGrainOverlay.tsx`**

```tsx
export function PaperGrainOverlay() {
  return <div className="grain-overlay" aria-hidden="true" />;
}
```

- [x] **Step 2: Wire it into `__root.tsx`**

```tsx
import { Outlet } from "react-router";
import { PaperGrainOverlay } from "@/shared/layout/PaperGrainOverlay";

export default function RootLayout() {
  return (
    <div className="min-h-screen bg-paper text-ink">
      <PaperGrainOverlay />
      <Outlet />
    </div>
  );
}
```

- [x] **Step 3: Verify**

Run: `cd web && npm run dev`. Open `/library`. DevTools shows `<div class="grain-overlay">` over the content, and the content stays interactive (the overlay does not block clicks). Ctrl+C.

- [x] **Step 4: Commit**

```bash
git add web/src/shared/layout/PaperGrainOverlay.tsx web/src/routes/__root.tsx
git commit -m "feat(web): add paper grain overlay to root layout"
```

---

### Task 9: The Sidebar component (a skeleton with nav items)

**Files:**
- Create: `web/src/shared/layout/Sidebar.tsx`
- Create: `web/src/shared/layout/Sidebar.test.tsx` (the test is added in Task 16, once vitest is configured)

- [x] **Step 1: Create `web/src/shared/layout/Sidebar.tsx`**

```tsx
import { NavLink } from "react-router";
import { cn } from "@/shared/lib/cn";

const navItems = [
  { to: "/library", label: "Library", number: "01" },
  { to: "/radar", label: "Radar", number: "02", disabled: true },
  { to: "/settings", label: "Settings", number: "03" },
];

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <aside className="flex h-full w-[280px] flex-col bg-paper-2 border-r border-rule">
      <div className="px-6 py-8 border-b border-rule">
        <p className="font-display italic text-3xl text-ink leading-none">Linktheca</p>
        <p className="label-sc mt-2 text-muted">A private archive</p>
      </div>

      <nav className="flex-1 px-6 py-6">
        <ul className="flex flex-col gap-3">
          {navItems.map((item) => (
            <li key={item.to}>
              {item.disabled ? (
                <span
                  className={cn(
                    "nav-item flex items-baseline gap-3 px-4 py-2 cursor-not-allowed opacity-50",
                  )}
                >
                  <span className="font-mono text-xs text-muted">{item.number}</span>
                  <span className="nav-label font-display text-lg">{item.label}</span>
                  <span className="label-sc text-muted ml-auto">soon</span>
                </span>
              ) : (
                <NavLink
                  to={item.to}
                  onClick={onNavigate}
                  className={({ isActive }) =>
                    cn(
                      "nav-item flex items-baseline gap-3 px-4 py-2 hover:text-ink",
                      isActive && "active",
                    )
                  }
                >
                  <span className="nav-number font-mono text-xs text-muted">{item.number}</span>
                  <span className="nav-label font-display text-lg text-ink-3">{item.label}</span>
                </NavLink>
              )}
            </li>
          ))}
        </ul>
      </nav>

      <div className="px-6 py-4 border-t border-rule">
        <p className="label-sc text-muted">v0.1.0 · self-hosted</p>
      </div>
    </aside>
  );
}
```

- [x] **Step 2: Commit (the test comes after the Vitest setup)**

```bash
git add web/src/shared/layout/Sidebar.tsx
git commit -m "feat(web): add Sidebar with nav items (Radar disabled stub)"
```

---

### Task 10: Topbar component

**Files:**
- Create: `web/src/shared/layout/Topbar.tsx`

- [x] **Step 1: Create `web/src/shared/layout/Topbar.tsx`**

```tsx
import { Menu, Plus } from "lucide-react";

type Props = {
  onMenuClick: () => void;
};

export function Topbar({ onMenuClick }: Props) {
  return (
    <header className="sticky top-0 z-10 h-16 bg-paper-2 border-b border-rule flex items-center px-4 lg:px-6">
      <button
        type="button"
        onClick={onMenuClick}
        aria-label="Open navigation"
        className="icon-btn lg:hidden"
      >
        <Menu className="h-5 w-5" strokeWidth={1.5} />
      </button>

      <div className="ml-auto flex items-center gap-3">
        <button
          type="button"
          aria-label="Add link"
          className="icon-btn"
          onClick={() => {
            // wired up in Library plan
            console.warn("Add Link not implemented yet");
          }}
        >
          <Plus className="h-5 w-5" strokeWidth={1.5} />
        </button>

        <div
          className="h-9 w-9 rounded-none border border-rule bg-paper flex items-center justify-center"
          aria-label="User menu placeholder"
        >
          <span className="font-display text-lg text-ink">L</span>
        </div>
      </div>
    </header>
  );
}
```

- [x] **Step 2: Commit**

```bash
git add web/src/shared/layout/Topbar.tsx
git commit -m "feat(web): add Topbar with hamburger and add-link button stubs"
```

---

### Task 11: MobileDrawer component

**Files:**
- Create: `web/src/shared/layout/MobileDrawer.tsx`

- [x] **Step 1: Create `web/src/shared/layout/MobileDrawer.tsx`**

```tsx
import * as Dialog from "@radix-ui/react-dialog";
import { Sidebar } from "@/shared/layout/Sidebar";
import { cn } from "@/shared/lib/cn";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function MobileDrawer({ open, onOpenChange }: Props) {
  const close = () => onOpenChange(false);
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          className={cn(
            "modal-backdrop fixed inset-0 z-30 lg:hidden",
            "data-[state=open]:animate-fade-in",
          )}
        />
        <Dialog.Content
          className={cn(
            "fixed inset-y-0 left-0 z-40 w-[280px] focus:outline-none lg:hidden",
            "data-[state=open]:animate-fade-in",
          )}
          aria-describedby={undefined}
        >
          <Dialog.Title className="sr-only">Navigation</Dialog.Title>
          <Sidebar onNavigate={close} />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
```

- [x] **Step 2: Commit**

```bash
git add web/src/shared/layout/MobileDrawer.tsx
git commit -m "feat(web): add MobileDrawer hosting Sidebar on <lg via Radix Dialog"
```

---

### Task 12: AppShell composition

**Files:**
- Create: `web/src/shared/layout/AppShell.tsx`

- [x] **Step 1: Create `web/src/shared/layout/AppShell.tsx`**

```tsx
import { useState } from "react";
import { Sidebar } from "@/shared/layout/Sidebar";
import { Topbar } from "@/shared/layout/Topbar";
import { MobileDrawer } from "@/shared/layout/MobileDrawer";

type Props = {
  children: React.ReactNode;
};

export function AppShell({ children }: Props) {
  const [drawerOpen, setDrawerOpen] = useState(false);

  return (
    <div className="min-h-screen">
      <div className="hidden lg:block fixed inset-y-0 left-0 z-20">
        <Sidebar />
      </div>

      <MobileDrawer open={drawerOpen} onOpenChange={setDrawerOpen} />

      <div className="lg:pl-[280px] flex flex-col min-h-screen">
        <Topbar onMenuClick={() => setDrawerOpen(true)} />
        <main className="flex-1">{children}</main>
      </div>
    </div>
  );
}
```

- [x] **Step 2: Wire AppShell in through `__app.tsx`** (a route layout)

Change `web/src/routes/__app.tsx`:

```tsx
import { Outlet } from "react-router";
import { AppShell } from "@/shared/layout/AppShell";

export default function AppLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  );
}
```

`__root.tsx` stays as it is (paper grain plus Outlet). The route structure already separates the public branch from the app branch, so no conditional logic is needed in the layout.

- [x] **Step 3: Verify**

Run: `cd web && npm run dev`. On `/library`:
- In a wide window (≥1024px) the sidebar is pinned on the left, the topbar is on top, and the content sits to the right of the sidebar.
- Narrow the window below 1024px — the sidebar disappears and a hamburger appears in the topbar. Clicking the hamburger opens the drawer from the left. Clicking a nav item inside closes the drawer and navigates.
- On `/login` — the public layout, with no sidebar or topbar and a centred card.

Ctrl+C.

- [x] **Step 4: Commit**

```bash
git add web/src/shared/layout/AppShell.tsx web/src/routes/__app.tsx
git commit -m "feat(web): compose AppShell with responsive sidebar and mount via __app layout"
```

---

### Task 13: PageHeader component

**Files:**
- Create: `web/src/shared/layout/PageHeader.tsx`
- Modify: `web/src/routes/library._index.tsx`
- Modify: `web/src/routes/settings.tsx`

- [x] **Step 1: Create `web/src/shared/layout/PageHeader.tsx`**

```tsx
type Props = {
  title: string;
  subtitle?: string;
  actions?: React.ReactNode;
};

export function PageHeader({ title, subtitle, actions }: Props) {
  return (
    <header className="px-4 lg:px-8 pt-10 pb-6 border-b border-rule">
      <div className="flex items-end justify-between gap-6 flex-wrap">
        <div>
          <h1 className="display-tight text-5xl lg:text-6xl text-ink">{title}</h1>
          {subtitle && <p className="label-sc mt-3 text-muted">{subtitle}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
    </header>
  );
}
```

- [x] **Step 2: Use it in `library._index.tsx`**

```tsx
import { PageHeader } from "@/shared/layout/PageHeader";

export default function LibraryListRoute() {
  return (
    <div>
      <PageHeader title="Library" subtitle="Your saved articles" />
      <div className="p-4 lg:p-8">
        <p className="font-body">Item list goes here.</p>
      </div>
    </div>
  );
}
```

- [x] **Step 3: Use it in `settings.tsx`**

```tsx
import { PageHeader } from "@/shared/layout/PageHeader";

export default function SettingsRoute() {
  return (
    <div>
      <PageHeader title="Settings" subtitle="Coming soon" />
    </div>
  );
}
```

- [x] **Step 4: Verify**

Run: `cd web && npm run dev`. On `/library` and `/settings` there is a large Cormorant title, a small-caps subtitle, and a divider below. Ctrl+C.

- [x] **Step 5: Commit**

```bash
git add web/src/shared/layout/PageHeader.tsx web/src/routes/library._index.tsx web/src/routes/settings.tsx
git commit -m "feat(web): add PageHeader and apply to Library/Settings routes"
```

---

## Group 4 — Data infrastructure

### Task 14: TanStack Query setup

**Files:**
- Modify: `web/package.json` (deps)
- Create: `web/src/shared/api/query-client.ts`
- Modify: `web/src/App.tsx`

- [x] **Step 1: Install TanStack Query**

Run: `cd web && npm install @tanstack/react-query`

- [x] **Step 2: Create `web/src/shared/api/query-client.ts`**

```ts
import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "@/shared/api/errors";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, err) =>
        err instanceof ApiError && err.status >= 500 ? failureCount < 2 : false,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
    mutations: { retry: false },
  },
});
```

(`errors.ts` does not exist yet — it is created in Task 17. Here we make a stub to be replaced. To break the cycle, create a minimal `errors.ts` now.)

- [x] **Step 3: Create a minimal `web/src/shared/api/errors.ts`**

```ts
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
```

(The full tests are in Task 17.)

- [x] **Step 4: Wrap App in `QueryClientProvider`**

```tsx
import { createBrowserRouter } from "react-router";
import { RouterProvider } from "react-router/dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/shared/api/query-client";
import RootLayout from "./routes/__root";
import PublicLayout from "./routes/_public";
import AppLayout from "./routes/__app";
import IndexRoute from "./routes/index";
import LoginRoute from "./routes/login";
import RegisterRoute from "./routes/register";
import LibraryListRoute from "./routes/library._index";
import LibraryItemRoute from "./routes/library.$id";
import SettingsRoute from "./routes/settings";
import NotFoundRoute from "./routes/not-found";

const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { index: true, element: <IndexRoute /> },
      {
        element: <PublicLayout />,
        children: [
          { path: "login", element: <LoginRoute /> },
          { path: "register", element: <RegisterRoute /> },
        ],
      },
      {
        element: <AppLayout />,
        children: [
          { path: "library", element: <LibraryListRoute /> },
          { path: "library/:id", element: <LibraryItemRoute /> },
          { path: "settings", element: <SettingsRoute /> },
        ],
      },
      { path: "*", element: <NotFoundRoute /> },
    ],
  },
]);

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
```

- [x] **Step 5: Verify build**

Run: `cd web && npm run build`
Expected: the build passes with no errors.

- [x] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/src/shared/api/ web/src/App.tsx
git commit -m "feat(web): wire TanStack Query client with retry/staleTime config"
```

---

### Task 15: Vitest + RTL + MSW setup

**Files:**
- Modify: `web/package.json` (deps + scripts)
- Create: `web/vitest.config.ts`
- Create: `web/src/test/setup.ts`
- Create: `web/src/test/sanity.test.ts` (smoke check)

- [x] **Step 1: Install the test dependencies**

Run:
```bash
cd web && npm install -D \
  vitest @vitest/ui \
  @testing-library/react @testing-library/jest-dom @testing-library/user-event \
  jsdom \
  msw
```

- [x] **Step 2: Create `web/vitest.config.ts`**

```ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    globals: true,
    css: false,
  },
});
```

- [x] **Step 3: Create `web/src/test/setup.ts`**

```ts
import "@testing-library/jest-dom/vitest";
import { afterAll, afterEach, beforeAll } from "vitest";
import { setupServer } from "msw/node";

export const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```

- [x] **Step 4: Add the test scripts to `web/package.json`**

Into the `scripts` block:

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [x] **Step 5: Create the smoke test `web/src/test/sanity.test.ts`**

```ts
import { describe, it, expect } from "vitest";

describe("test setup", () => {
  it("can run a basic assertion", () => {
    expect(1 + 1).toBe(2);
  });

  it("has DOM available", () => {
    const el = document.createElement("div");
    el.textContent = "hello";
    expect(el.textContent).toBe("hello");
  });
});
```

- [x] **Step 6: Run tests**

Run: `cd web && npm test`
Expected: 2 tests PASS.

- [x] **Step 7: Commit**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/test/
git commit -m "test(web): set up Vitest + RTL + MSW with jsdom environment"
```

---

### Task 16: A Sidebar render test (using the configured Vitest)

**Files:**
- Create: `web/src/shared/layout/Sidebar.test.tsx`

- [x] **Step 1: Write failing test `web/src/shared/layout/Sidebar.test.tsx`**

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { Sidebar } from "./Sidebar";

describe("Sidebar", () => {
  function renderWithRouter() {
    return render(
      <MemoryRouter>
        <Sidebar />
      </MemoryRouter>,
    );
  }

  it("renders the masthead", () => {
    renderWithRouter();
    expect(screen.getByText("Linktheca")).toBeInTheDocument();
  });

  it("renders Library and Settings as enabled nav links", () => {
    renderWithRouter();
    expect(screen.getByRole("link", { name: /library/i })).toHaveAttribute("href", "/library");
    expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute("href", "/settings");
  });

  it("renders Radar as disabled (no link)", () => {
    renderWithRouter();
    const radar = screen.getByText("Radar");
    expect(radar.closest("a")).toBeNull();
    expect(screen.getByText(/soon/i)).toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run tests**

Run: `cd web && npm test`
Expected: every Sidebar test passes, and the smoke tests pass too.

- [x] **Step 3: Commit**

```bash
git add web/src/shared/layout/Sidebar.test.tsx
git commit -m "test(web): cover Sidebar nav rendering and Radar disabled state"
```

---

### Task 17: ApiError + apiFetch (a skeleton, without refresh)

**Files:**
- Create: `web/src/shared/api/errors.test.ts` (tests for the `errors.ts` already created in Task 14)
- Create: `web/src/shared/api/client.ts`
- Create: `web/src/shared/api/client.test.ts`

- [x] **Step 1: A failing test for ApiError in `web/src/shared/api/errors.test.ts`**

```ts
import { describe, it, expect } from "vitest";
import { ApiError } from "./errors";

describe("ApiError", () => {
  it("captures status, code, message, and details", () => {
    const err = new ApiError(422, "validation_failed", "Invalid input", { field: "email" });
    expect(err.status).toBe(422);
    expect(err.code).toBe("validation_failed");
    expect(err.message).toBe("Invalid input");
    expect(err.details).toEqual({ field: "email" });
  });

  it("is an instance of Error", () => {
    const err = new ApiError(500, "internal", "boom");
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ApiError");
  });
});
```

- [x] **Step 2: Run the test (it should pass — `errors.ts` was created in Task 14)**

Run: `cd web && npm test src/shared/api/errors.test.ts`
Expected: PASS.

- [x] **Step 3: A failing test for `apiFetch` in `web/src/shared/api/client.test.ts`**

```ts
import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { apiFetch } from "./client";
import { ApiError } from "./errors";

describe("apiFetch", () => {
  beforeEach(() => {
    server.use(
      http.get("/api/echo", () =>
        HttpResponse.json({ ok: true }),
      ),
      http.get("/api/forbidden", () =>
        HttpResponse.json({ code: "forbidden", message: "Nope" }, { status: 403 }),
      ),
      http.get("/api/server-error", () =>
        HttpResponse.json({ code: "internal", message: "Boom" }, { status: 500 }),
      ),
      http.get("/api/no-json", () =>
        new HttpResponse("plain text", { status: 502 }),
      ),
    );
  });

  it("prefixes /api and parses JSON", async () => {
    const data = await apiFetch<{ ok: boolean }>("/echo");
    expect(data.ok).toBe(true);
  });

  it("throws ApiError for 4xx with code+message from body", async () => {
    await expect(apiFetch("/forbidden")).rejects.toMatchObject({
      status: 403,
      code: "forbidden",
      message: "Nope",
    } satisfies Partial<ApiError>);
  });

  it("throws ApiError for 5xx", async () => {
    await expect(apiFetch("/server-error")).rejects.toBeInstanceOf(ApiError);
  });

  it("throws ApiError with synthetic code for non-JSON error", async () => {
    await expect(apiFetch("/no-json")).rejects.toMatchObject({
      status: 502,
      code: "http_error",
    });
  });
});
```

- [x] **Step 4: Create `web/src/shared/api/client.ts`**

```ts
import { ApiError } from "./errors";

const API_BASE = "/api";

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type") && init?.body) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers,
  });

  if (res.ok) {
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  let code = "http_error";
  let message = res.statusText || "Request failed";
  let details: unknown;

  const ct = res.headers.get("content-type") ?? "";
  if (ct.includes("application/json")) {
    try {
      const body = (await res.json()) as { code?: string; message?: string; details?: unknown };
      if (typeof body.code === "string") code = body.code;
      if (typeof body.message === "string") message = body.message;
      details = body.details;
    } catch {
      // fall through with synthetic code
    }
  }

  throw new ApiError(res.status, code, message, details);
}
```

- [x] **Step 5: Run tests**

Run: `cd web && npm test`
Expected: all tests PASS.

- [x] **Step 6: Commit**

```bash
git add web/src/shared/api/errors.ts web/src/shared/api/errors.test.ts web/src/shared/api/client.ts web/src/shared/api/client.test.ts
git commit -m "feat(web): add apiFetch wrapper with ApiError normalization"
```

---

### Task 18: Zustand auth store

**Files:**
- Modify: `web/package.json` (deps)
- Create: `web/src/features/auth/store.ts`
- Create: `web/src/features/auth/store.test.ts`

- [x] **Step 1: Install Zustand**

Run: `cd web && npm install zustand`

- [x] **Step 2: Failing test `web/src/features/auth/store.test.ts`**

```ts
import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "./store";

describe("useAuthStore", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    useAuthStore.setState({ status: "bootstrapping" });
  });

  it("starts in bootstrapping status with no token or user", () => {
    const s = useAuthStore.getState();
    expect(s.status).toBe("bootstrapping");
    expect(s.accessToken).toBeNull();
    expect(s.user).toBeNull();
  });

  it("setSession transitions to authed and stores token+user", () => {
    useAuthStore.getState().setSession("tok-123", {
      id: 1,
      email: "a@b.c",
      displayName: "A",
      isAdmin: false,
    });
    const s = useAuthStore.getState();
    expect(s.status).toBe("authed");
    expect(s.accessToken).toBe("tok-123");
    expect(s.user?.email).toBe("a@b.c");
  });

  it("clearSession resets to anonymous", () => {
    useAuthStore.getState().setSession("t", { id: 1, email: "x", displayName: "X", isAdmin: false });
    useAuthStore.getState().clearSession();
    const s = useAuthStore.getState();
    expect(s.status).toBe("anonymous");
    expect(s.accessToken).toBeNull();
    expect(s.user).toBeNull();
  });
});
```

- [x] **Step 3: Create `web/src/features/auth/store.ts`**

```ts
import { create } from "zustand";

export type User = {
  id: number;
  email: string;
  displayName: string;
  isAdmin: boolean;
};

export type AuthStatus = "bootstrapping" | "authed" | "anonymous";

type AuthState = {
  accessToken: string | null;
  user: User | null;
  status: AuthStatus;
  setSession: (accessToken: string, user: User) => void;
  clearSession: () => void;
};

export const useAuthStore = create<AuthState>((set) => ({
  accessToken: null,
  user: null,
  status: "bootstrapping",
  setSession: (accessToken, user) => set({ accessToken, user, status: "authed" }),
  clearSession: () => set({ accessToken: null, user: null, status: "anonymous" }),
}));
```

- [x] **Step 4: Run tests**

Run: `cd web && npm test`
Expected: store tests PASS.

- [x] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/src/features/auth/
git commit -m "feat(web): add Zustand auth store with bootstrapping/authed/anonymous states"
```

---

### Task 19: Wire the access token from the auth store into apiFetch

**Files:**
- Modify: `web/src/shared/api/client.ts`
- Modify: `web/src/shared/api/client.test.ts`

- [x] **Step 1: A failing test — add cases to `client.test.ts`**

Add a new sub-describe at the end of the existing `describe("apiFetch", ...)`:

```ts
import { useAuthStore } from "@/features/auth/store";

describe("apiFetch with auth token", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    server.use(
      http.get("/api/echo-auth", ({ request }) => {
        const auth = request.headers.get("Authorization");
        return HttpResponse.json({ auth });
      }),
    );
  });

  it("omits Authorization header when no token", async () => {
    const r = await apiFetch<{ auth: string | null }>("/echo-auth");
    expect(r.auth).toBeNull();
  });

  it("sends Bearer token when set in store", async () => {
    useAuthStore.getState().setSession("tok-xyz", {
      id: 1,
      email: "a@b.c",
      displayName: "A",
      isAdmin: false,
    });
    const r = await apiFetch<{ auth: string | null }>("/echo-auth");
    expect(r.auth).toBe("Bearer tok-xyz");
  });
});
```

- [x] **Step 2: Run the test and see it FAIL**

Run: `cd web && npm test src/shared/api/client.test.ts`
Expected: the new "Bearer" tests FAIL.

- [x] **Step 3: Update `web/src/shared/api/client.ts`**

```ts
import { ApiError } from "./errors";
import { useAuthStore } from "@/features/auth/store";

const API_BASE = "/api";

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type") && init?.body) {
    headers.set("Content-Type", "application/json");
  }
  const token = useAuthStore.getState().accessToken;
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(`${API_BASE}${path}`, { ...init, headers });

  if (res.ok) {
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  let code = "http_error";
  let message = res.statusText || "Request failed";
  let details: unknown;

  const ct = res.headers.get("content-type") ?? "";
  if (ct.includes("application/json")) {
    try {
      const body = (await res.json()) as { code?: string; message?: string; details?: unknown };
      if (typeof body.code === "string") code = body.code;
      if (typeof body.message === "string") message = body.message;
      details = body.details;
    } catch {
      // fall through
    }
  }

  throw new ApiError(res.status, code, message, details);
}
```

- [x] **Step 4: Run tests**

Run: `cd web && npm test`
Expected: all tests PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/shared/api/client.ts web/src/shared/api/client.test.ts
git commit -m "feat(web): inject Bearer token from auth store into apiFetch"
```

---

## Group 5 — Tooling and deploy

### Task 20: ESLint + Prettier

**Files:**
- Modify: `web/package.json` (deps + scripts)
- Create: `web/eslint.config.js`
- Create: `web/.prettierrc`
- Create: `web/.prettierignore`

- [x] **Step 1: Install the dependencies**

Run:
```bash
cd web && npm install -D \
  eslint@^9 @eslint/js@^9 typescript-eslint \
  eslint-plugin-react eslint-plugin-react-hooks eslint-plugin-jsx-a11y \
  prettier eslint-config-prettier
```

> ESLint v10 is not yet supported by `eslint-plugin-jsx-a11y` (≤v9) or `eslint-plugin-react` (≤v9.7+); `*` resolves to v10 and npm fails with ERESOLVE. We pin `eslint`/`@eslint/js` to `^9` until compatible plugin releases land.

- [x] **Step 2: Create `web/eslint.config.js`**

```js
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import jsxA11y from "eslint-plugin-jsx-a11y";
import prettier from "eslint-config-prettier";

export default tseslint.config(
  { ignores: ["dist", "node_modules"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  react.configs.flat.recommended,
  react.configs.flat["jsx-runtime"],
  reactHooks.configs.flat.recommended,
  jsxA11y.flatConfigs.recommended,
  {
    settings: {
      react: { version: "detect" },
    },
  },
  {
    files: ["**/*.{ts,tsx}"],
    rules: {
      "react/prop-types": "off",
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_" }],
    },
  },
  prettier,
);
```

> We use the plugins' flat-config presets (`react.configs.flat.*`, `reactHooks.configs.flat.recommended`, `jsxA11y.flatConfigs.recommended`) — the old `configs.recommended.rules` form is legacy eslintrc and is effectively absent from `eslint-plugin-react-hooks@7`. `jsx-runtime` switches off `react-in-jsx-scope` by itself. `prop-types` is off — TypeScript covers the types.

> `settings.react.version` sits in its own block with no `files` restriction: `react.configs.flat.recommended` applies to every file, and without a global `settings` ESLint emits a "React version not specified" warning on `npm run lint`.

- [x] **Step 3: Create `web/.prettierrc`**

```json
{
  "printWidth": 100,
  "trailingComma": "all",
  "singleQuote": false,
  "semi": true
}
```

- [x] **Step 4: Create `web/.prettierignore`**

```
dist
node_modules
package-lock.json
```

- [x] **Step 5: Add the scripts to `web/package.json`**

```json
"lint": "eslint . --max-warnings 0",
"typecheck": "tsc --noEmit",
"format": "prettier --write ."
```

- [x] **Step 6: Verify**

Run: `cd web && npm run typecheck && npm run lint`
Expected: both PASS. If lint complains about rule violations, fix them precisely (without suppressing the rules).

- [x] **Step 7: Run prettier once to normalize the style**

Run: `cd web && npm run format`
Expected: some files are rewritten.

- [x] **Step 8: Verify lint after formatting**

Run: `cd web && npm run lint && npm test`
Expected: PASS.

- [x] **Step 9: Commit**

```bash
git add web/package.json web/package-lock.json web/eslint.config.js web/.prettierrc web/.prettierignore web/src/
git commit -m "chore(web): set up ESLint flat config + Prettier and run initial format"
```

---

### Task 21: Production Dockerfile + nginx.conf

**Files:**
- Create: `web/Dockerfile`
- Create: `web/nginx.conf`
- Create: `web/.dockerignore`

- [x] **Step 1: Create `web/Dockerfile`**

```dockerfile
FROM node:24-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

- [x] **Step 2: Create `web/nginx.conf`**

`resolver 127.0.0.11` plus a variable-based `proxy_pass` are needed so nginx starts without a live `backend` (otherwise it dies resolving the upstream at startup). The `rewrite` strips the `/api/` prefix — the backend expects clean paths (see `vite.config.ts`: the dev proxy does the same).

```
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    resolver 127.0.0.11 valid=10s ipv6=off;

    location / {
        try_files $uri /index.html;
    }

    location /api/ {
        set $backend_upstream http://backend:8080;
        rewrite ^/api/(.*)$ /$1 break;
        proxy_pass $backend_upstream;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    location = /index.html {
        add_header Cache-Control "no-store";
    }
}
```

- [x] **Step 3: Create `web/.dockerignore`**

```
node_modules
dist
.env.local
*.log
.git
```

- [x] **Step 4: Verify the build (requires docker)**

Run: `cd web && docker build -t linktheca-web:test .`
Expected: the build passes with no errors (two stages, with the final image on nginx:alpine).

- [x] **Step 5: A smoke run of the container**

Run: `docker run --rm -d --name linktheca-web-test -p 8090:80 linktheca-web:test`
Then: `curl -sI http://localhost:8090/` → expect `200 OK` and `text/html`.
Then: `docker rm -f linktheca-web-test`.

- [x] **Step 6: Commit**

```bash
git add web/Dockerfile web/nginx.conf web/.dockerignore
git commit -m "feat(web): add multi-stage Dockerfile and nginx config for SPA + /api proxy"
```

---

### Task 22: `compose.prod.yaml`

**Files:**
- Create: `compose.prod.yaml` (at the repo root)

- [x] **Step 1: Create `compose.prod.yaml` at the repo root**

```yaml
services:
  postgres:
    image: pgvector/pgvector:0.8.2-pg18-trixie
    restart: unless-stopped
    environment:
      POSTGRES_USER: linktheca
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?required}
      POSTGRES_DB: linktheca
    volumes:
      - linktheca_pg_data:/var/lib/postgresql

  tei:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.9
    command: ["--model-id", "BAAI/bge-m3", "--port", "8080"]
    volumes:
      - linktheca_tei_data:/data

  backend:
    build:
      context: .
      dockerfile: Dockerfile
    depends_on:
      - postgres
      - tei
    environment:
      LINKTHECA_DB_DSN: postgres://linktheca:${POSTGRES_PASSWORD}@postgres:5432/linktheca?sslmode=disable
      LINKTHECA_JWT_SECRET: ${JWT_SECRET:?required}
      LINKTHECA_TEI_URL: http://tei:8080
    expose:
      - "8080"

  web:
    build:
      context: ./web
      dockerfile: Dockerfile
    depends_on:
      - backend
    ports:
      - "80:80"

volumes:
  linktheca_pg_data:
  linktheca_tei_data:
```

**Note:** the root `Dockerfile` for the backend is not created by this plan (that is outside a frontend plan's scope). If it does not exist yet, this compose file is incorrect until the backend Dockerfile appears. That is acceptable — `compose.prod.yaml` pins the target configuration, not a deployable-as-is state.

- [x] **Step 2: Verify compose syntax**

Run: `docker compose -f compose.prod.yaml config`
Expected: compose validates (it may complain about missing environment variables — that is fine for a syntax check; run it with dummy values exported if needed: `POSTGRES_PASSWORD=p JWT_SECRET=s docker compose -f compose.prod.yaml config`).

- [x] **Step 3: Commit**

```bash
git add compose.prod.yaml
git commit -m "feat(deploy): add compose.prod.yaml wiring web + backend + postgres + tei"
```

---

### Task 23: Extend the CI workflow

**Files:**
- Modify: `.github/workflows/ci.yml`

- [x] **Step 1: Read current `.github/workflows/ci.yml`**

You already know the current workflow with its single `backend` job. We add a second job, `frontend`.

- [x] **Step 2: Change `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
          cache: true

      - name: go vet
        run: go vet ./...

      - name: go test (short, unit only)
        run: go test ./... -race -count=1 -short

      - name: go test (integration with testcontainers)
        run: go test ./... -race -count=1

  frontend:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '24'
          cache: npm
          cache-dependency-path: web/package-lock.json

      - name: install
        run: npm ci

      - name: lint
        run: npm run lint

      - name: typecheck
        run: npm run typecheck

      - name: test
        run: npm test

      - name: build
        run: npm run build
```

- [x] **Step 3: Verify locally before pushing**

Run:
```bash
cd web && npm ci && npm run lint && npm run typecheck && npm test && npm run build
```
Expected: every step passes — this is exactly what CI will run.

- [x] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add frontend job (lint, typecheck, test, build) on Node 24"
```

---

### Task 24: A final sanity check of the plan

- [x] **Step 1: Check the whole dev workflow from scratch**

```bash
# in one terminal
make dev-db && make run

# in another terminal
cd web && npm run dev
```

Open `http://localhost:5173`:
- The landing page redirects to `/library`.
- The sidebar is pinned on the left on desktop.
- Narrow the window below 1024 — the sidebar disappears and the hamburger opens the drawer.
- `/login`, `/register` — the public layout with no shell.
- A 404 on a nonexistent path.
- DevTools Network: `/api/healthz` → 200 ok (through the Vite proxy).

- [x] **Step 2: Run the CI equivalent locally**

```bash
cd web && npm run lint && npm run typecheck && npm test && npm run build
```

- [x] **Step 3: If anything is broken, fix it and commit that separately**

No TDD ritual for each micro-fix — this is plan polish, not a feature step.

---

## Consolidated self-review

**Spec coverage check:**

| Spec section | Tasks |
|---|---|
| 1. Foundation: setup, structure, tokens | T1 (Vite/TS), T2 (Tailwind+tokens), T3 (fonts), T4 (utility classes), T5 (proxy) |
| 2. AppShell and responsive layout | T8 (PaperGrainOverlay), T9 (Sidebar), T10 (Topbar), T11 (MobileDrawer), T12 (AppShell), T13 (PageHeader), the `_public` layout — T7 |
| 3. API client and auth state | T14 (Query client), T17 (apiFetch+ApiError), T18 (auth store), T19 (Bearer wiring); the refresh flow and bootstrap are in the next plan, Auth |
| 4. Routing, Auth screens, Library screens | T7 (route tree + stubs); the full screens come in the following plans |
| 5. Testing | T15 (Vitest+RTL+MSW), T16 (Sidebar test), T17 (apiFetch tests), T18 (store tests) |
| 6. Build and deploy | T21 (Dockerfile+nginx), T22 (compose.prod.yaml), T23 (CI) |

Not covered by this plan (by design): the refresh flow in `apiFetch`, ProtectedRoute, bootstrap-on-mount, the login/register forms, and library list/reader/add-link/edit/delete — those belong to the following Auth and Library plans.

**Placeholder scan:** concrete code in every step, with all commit messages and `expected output` stated explicitly.

**Type consistency:** `User`, `AuthState`, `AuthStatus`, and `ApiError` are defined once, and the later tasks reference those names consistently.

---

## Next steps

After this whole plan is executed — two further plans:

1. `2026-05-XX-frontend-auth.md` — the refresh flow in `apiFetch` (a singleton Promise), bootstrap-on-mount, ProtectedRoute, the login/register forms (RHF+Zod), the `/auth/me` integration, and logout.
2. `2026-05-XX-frontend-library.md` — `features/library/api.ts`, the list (filters/pagination/empty/loading/error), the Add Link modal with three-stage progress, the Reader view (drop cap, reading progress, mark-as-read on scroll), edit (state/favorite/note autosave), delete with a confirmation, optimistic updates, and sonner toasts.
