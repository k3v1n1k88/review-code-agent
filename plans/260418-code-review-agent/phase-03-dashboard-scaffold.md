# Phase 03 — Next.js Admin Dashboard Scaffold

## Context Links
- Parent: [plan.md](plan.md)
- Research: none direct; design session (admin dashboard requirement)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 6h
- **Group**: A (parallel — no deps)
- **Description**: Scaffold Next.js 15 (App Router, Turbopack) admin. Auth pages (login), protected shell, API client factory pointing at Go backend. Provide empty route stubs for `/teams`, `/models`, `/reviews`, `/metrics`, `/usage`, `/feedback` — Phase 06/10/12 fill them.

## Key Insights
- App Router + Server Components for listings, Client Components for forms.
- Use shadcn/ui + Tailwind — keeps files small, promotes composition.
- Single API client wrapper with Bearer-token handling (JWT from login) in `web/lib/api/client.ts`.
- NO backend-specific logic here — pure UI shell + routing.

## Requirements

### Functional
- `/login` page: username+password form, posts to `/api/v1/auth/login`, stores JWT.
- Layout: sidebar with links, header with user menu, theme toggle.
- Protected route wrapper: redirects to `/login` if no token.
- Route stubs for: Teams, Models, Reviews, Metrics, Usage, Feedback.
- 404 + error boundary pages.

### Non-functional
- Node 20+, Next 15+, TypeScript strict mode.
- Lighthouse score > 90 on login page.
- Build time < 60s in CI.

## Architecture

```
web/
├── app/
│   ├── layout.tsx              # Root: providers (theme, query)
│   ├── page.tsx                # Redirects to /reviews or /login
│   ├── login/page.tsx
│   ├── (protected)/
│   │   ├── layout.tsx          # Sidebar + header
│   │   ├── teams/page.tsx      # stub
│   │   ├── models/page.tsx     # stub
│   │   ├── reviews/page.tsx    # stub
│   │   ├── metrics/page.tsx    # stub
│   │   ├── usage/page.tsx      # stub
│   │   └── feedback/page.tsx   # stub
│   ├── not-found.tsx
│   └── error.tsx
├── components/
│   ├── ui/                     # shadcn primitives
│   │   ├── button.tsx
│   │   ├── input.tsx
│   │   ├── card.tsx
│   │   └── sidebar.tsx
│   ├── auth/
│   │   └── login-form.tsx
│   └── shell/
│       ├── sidebar-nav.tsx
│       └── user-menu.tsx
├── lib/
│   ├── api/
│   │   ├── client.ts           # fetch wrapper w/ auth header
│   │   └── types.ts            # shared types (Review, Team, etc)
│   ├── auth/
│   │   ├── token.ts            # localStorage wrapper
│   │   └── guard.tsx           # client HOC
│   └── config.ts               # env reader
├── public/
├── package.json
├── next.config.ts
├── tailwind.config.ts
├── tsconfig.json
└── Dockerfile
```

## Related Code Files

### Create
- `/web/package.json`
- `/web/next.config.ts`
- `/web/tailwind.config.ts`
- `/web/tsconfig.json`
- `/web/postcss.config.mjs`
- `/web/Dockerfile` (multi-stage, `output: 'standalone'`)
- `/web/app/layout.tsx`
- `/web/app/page.tsx`
- `/web/app/login/page.tsx`
- `/web/app/(protected)/layout.tsx`
- `/web/app/(protected)/{teams,models,reviews,metrics,usage,feedback}/page.tsx` (6 stubs)
- `/web/app/not-found.tsx`, `/web/app/error.tsx`
- `/web/components/ui/{button,input,card,sidebar,form}.tsx`
- `/web/components/auth/login-form.tsx`
- `/web/components/shell/{sidebar-nav,user-menu}.tsx`
- `/web/lib/api/client.ts`
- `/web/lib/api/types.ts`
- `/web/lib/auth/{token,guard}.tsx`
- `/web/lib/config.ts`

### Modify
- `/docker-compose.yml` already references `web` service (from Phase 01).

## Implementation Steps

1. **Init project**:
   ```
   npx create-next-app@latest web --typescript --tailwind --app --src-dir=false --import-alias "@/*"
   cd web
   npx shadcn@latest init
   npx shadcn@latest add button input card form sonner
   npm i @tanstack/react-query zod react-hook-form
   ```

2. **`lib/config.ts`**:
   ```ts
   export const config = {
     apiUrl: process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080",
   };
   ```

3. **`lib/api/client.ts`** (<100 lines):
   ```ts
   import { getToken } from "@/lib/auth/token";
   import { config } from "@/lib/config";

   export class ApiError extends Error { constructor(public status: number, msg: string){super(msg);} }

   export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
     const token = getToken();
     const res = await fetch(`${config.apiUrl}${path}`, {
       ...init,
       headers: {
         "Content-Type": "application/json",
         ...(token ? { Authorization: `Bearer ${token}` } : {}),
         ...(init.headers ?? {}),
       },
     });
     if (!res.ok) throw new ApiError(res.status, await res.text());
     return res.json() as Promise<T>;
   }
   ```

4. **`lib/auth/token.ts`**:
   ```ts
   const KEY = "cra_token";
   export const getToken = () => typeof window !== "undefined" ? localStorage.getItem(KEY) : null;
   export const setToken = (t: string) => localStorage.setItem(KEY, t);
   export const clearToken = () => localStorage.removeItem(KEY);
   ```

5. **`lib/auth/guard.tsx`**: client-component wrapper; redirects to `/login` if no token.

6. **`app/login/page.tsx`** + **`components/auth/login-form.tsx`**:
   ```tsx
   // login-form.tsx
   "use client";
   // react-hook-form + zod, posts /api/v1/auth/login, stores JWT, routes to /reviews
   ```

7. **`app/(protected)/layout.tsx`**: uses `Guard`, renders sidebar + outlet.

8. **`components/shell/sidebar-nav.tsx`**:
   ```tsx
   const items = [
     { href: "/reviews", label: "Reviews" },
     { href: "/feedback", label: "Feedback" },
     { href: "/metrics", label: "Metrics" },
     { href: "/usage", label: "Usage" },
     { href: "/models", label: "Models" },
     { href: "/teams", label: "Teams" },
   ];
   ```

9. **Six route stubs**:
   ```tsx
   export default function Page() { return <div className="p-6"><h1>Teams</h1><p>Coming in Phase 06.</p></div>; }
   ```

10. **Dockerfile** (multi-stage, standalone):
    ```dockerfile
    FROM node:20-alpine AS deps
    WORKDIR /app
    COPY package.json package-lock.json ./
    RUN npm ci

    FROM node:20-alpine AS build
    WORKDIR /app
    COPY --from=deps /app/node_modules ./node_modules
    COPY . .
    RUN npm run build

    FROM node:20-alpine AS run
    WORKDIR /app
    ENV NODE_ENV=production
    COPY --from=build /app/.next/standalone ./
    COPY --from=build /app/.next/static ./.next/static
    COPY --from=build /app/public ./public
    EXPOSE 3000
    CMD ["node", "server.js"]
    ```
    Set `output: 'standalone'` in `next.config.ts`.

## Todo List

- [ ] `create-next-app` completed; builds with `npm run build`
- [ ] shadcn primitives installed (`button`, `input`, `card`, `form`, `sonner`)
- [ ] `lib/api/client.ts` typed, uses JWT
- [ ] Login form with zod schema (username/password)
- [ ] Protected route guard redirects when no token
- [ ] Sidebar links to 6 stubs
- [ ] 404 + error boundary pages
- [ ] Dockerfile builds; image < 150MB
- [ ] `docker compose up web` serves `http://localhost:3000`

## Success Criteria
- Visiting `/teams` without token redirects to `/login`.
- `npm run build` produces standalone output.
- `npm run lint` clean.

## Risk Assessment
- **Backend not ready**: login form posts to `/api/v1/auth/login` — Phase 04 provides endpoint. Mock with MSW during dev if backend unavailable.
- **Build size**: keep dependencies minimal; lazy-load charts (Phase 12) and markdown renderers (Phase 10).

## Security Considerations
- JWT in `localStorage` is XSS-exposed; acceptable for internal tool. Escalate to HttpOnly cookie if moved external (Phase 14 follow-up).
- No service-to-service calls from browser; all via API client with Bearer.

## Next Steps
- **Unblocks**: Phase 06 (fills stubs), Phase 10 (feedback UI), Phase 12 (usage dashboard).
- **Parallel**: Phase 01, 02.
