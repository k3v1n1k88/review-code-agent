# Phase 04 — Go Backend Core (Echo, Auth, Guardrails)

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-01-backend-report.md` §6 (rate limit/validation/secrets), §7 (middleware), §9 (DI patterns)
- Depends on: Phase 01 (scaffold), Phase 02 (schema)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 10h
- **Group**: B (parallel with 05, 06)
- **Description**: Wire Echo server with full middleware stack, domain entities, repositories (Postgres), auth (login + API key), guardrail layer (rate limit + validation + secrets filter). No review logic yet — Phase 05 plugs in.

## Key Insights
- Research §2 clean-arch layering enforced: domain never imports infrastructure.
- Dual auth: JWT (dashboard users) OR API key (webhook/REST callers). Both emit a `RequestPrincipal` on context.
- Guardrail runs BEFORE review handoff: rate-limit → validate → secrets scan → enqueue.
- Rate limit per-team (API key) and per-IP (login brute-force). **Redis-backed from day 1** (validated decision) using `go-redis/redis-rate`. Redis container provided in Phase 01.
<!-- Updated: Validation Session 1 - Redis rate limiter, not in-memory -->

## Requirements

### Functional
- `POST /api/v1/auth/login` → JWT (15min) + refresh (7d).
- `POST /api/v1/auth/refresh`.
- `GET /api/v1/me` (any auth).
- Middleware chain: RequestID → Logger → Recover → CORS → RateLimit → Auth (route-specific) → Validation.
- Domain entities + repository interfaces defined.
- Postgres implementations for: teams, users, api_keys.
- Guardrail package: `guardrail.Check(ctx, diff) error`.

### Non-functional
- p50 login latency < 50ms.
- Rate limit: 60 req/min/IP on `/auth/*`, 1000 req/min/API-key on `/api/*`.
- Structured slog JSON output.

## Architecture

```
internal/
├── domain/
│   ├── team.go, user.go, apikey.go, review.go (stubs), errors.go
│   └── principal.go             # RequestPrincipal (TeamID, UserID, AuthKind)
├── repository/
│   └── postgres/
│       ├── db.go                # pgx pool setup
│       ├── team_repo.go
│       ├── user_repo.go
│       └── apikey_repo.go
├── usecase/
│   └── auth/
│       ├── login.go
│       ├── refresh.go
│       └── apikey.go            # issue/revoke
├── infrastructure/
│   └── auth/
│       ├── jwt.go               # sign/parse
│       ├── password.go          # bcrypt wrappers
│       └── apikey.go            # token gen + hash
├── delivery/
│   └── http/
│       ├── middleware/
│       │   ├── requestid.go
│       │   ├── logger.go
│       │   ├── ratelimit.go
│       │   ├── cors.go
│       │   ├── auth_jwt.go
│       │   └── auth_apikey.go
│       ├── handlers/
│       │   ├── auth_handler.go
│       │   └── health_handler.go
│       └── routes.go
└── guardrail/
    ├── guardrail.go              # aggregator
    ├── secrets.go                # regex from research §6
    ├── size.go                   # diff size/complexity
    └── validate.go               # ozzo wrappers
```

## Related Code Files

### Create
- `/internal/domain/{team,user,apikey,review,principal,errors}.go`
- `/internal/repository/postgres/{db,team_repo,user_repo,apikey_repo}.go`
- `/internal/usecase/auth/{login,refresh,apikey}.go`
- `/internal/infrastructure/auth/{jwt,password,apikey}.go`
- `/internal/delivery/http/middleware/{requestid,logger,ratelimit,cors,auth_jwt,auth_apikey}.go`
- `/internal/delivery/http/handlers/{auth_handler,health_handler}.go`
- `/internal/delivery/http/routes.go`
- `/internal/guardrail/{guardrail,secrets,size,validate}.go`

### Modify
- `/cmd/server/main.go` — wire dependencies, register routes.
- `/internal/config/config.go` — add `RateLimit` knobs.

## Implementation Steps

1. **Domain entities** (`internal/domain/team.go` etc). Each file < 80 lines, tags-only structs, no logic.
   ```go
   // principal.go
   type AuthKind string
   const (AuthJWT AuthKind="jwt"; AuthAPIKey AuthKind="apikey")
   type Principal struct {
       TeamID, UserID uuid.UUID
       Kind AuthKind
       APIKeyID uuid.UUID // if apikey
   }
   ```

2. **Postgres pool** (`repository/postgres/db.go`):
   ```go
   func NewPool(ctx context.Context, cfg config.DBConfig) (*pgxpool.Pool, error) {
       pc, err := pgxpool.ParseConfig(cfg.DSN)
       if err != nil { return nil, err }
       pc.MaxConns = int32(cfg.MaxOpenConns)
       return pgxpool.NewWithConfig(ctx, pc)
   }
   ```

3. **Repository interfaces in `domain/`, implementations in `repository/postgres/`** — no SQL in domain.
   ```go
   // domain/user.go
   type UserRepo interface {
       FindByUsername(ctx context.Context, username string) (*User, error)
       Create(ctx context.Context, u *User) error
   }
   ```

4. **Password + JWT**:
   ```go
   // infrastructure/auth/password.go
   func Hash(pw string) (string, error) { b, e := bcrypt.GenerateFromPassword([]byte(pw), 12); return string(b), e }
   func Compare(hash, pw string) bool { return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil }

   // infrastructure/auth/jwt.go
   type Claims struct { TeamID, UserID string; jwt.RegisteredClaims }
   func Sign(secret []byte, c Claims, ttl time.Duration) (string, error) { ... }
   func Parse(secret []byte, token string) (*Claims, error) { ... }
   ```

5. **API key token**:
   ```go
   // infrastructure/auth/apikey.go
   // Token format: "cra_" + 32 random bytes base64url (43 chars)
   // Stored: sha256(token || pepper)
   // Prefix: first 8 chars of token after "cra_"
   func Generate(pepper string) (token, prefix, hash string)
   func Hash(token, pepper string) string
   ```

6. **Middleware**:

   `middleware/auth_jwt.go`:
   ```go
   func JWT(secret []byte) echo.MiddlewareFunc {
       return func(next echo.HandlerFunc) echo.HandlerFunc {
           return func(c echo.Context) error {
               h := c.Request().Header.Get("Authorization")
               if !strings.HasPrefix(h, "Bearer ") { return echo.ErrUnauthorized }
               claims, err := authinfra.Parse(secret, strings.TrimPrefix(h, "Bearer "))
               if err != nil { return echo.ErrUnauthorized }
               c.Set("principal", &domain.Principal{TeamID: uuid.MustParse(claims.TeamID), UserID: uuid.MustParse(claims.UserID), Kind: domain.AuthJWT})
               return next(c)
           }
       }
   }
   ```

   `middleware/auth_apikey.go` — parse header `X-API-Key`, look up by prefix, compare hash, set principal.

   `middleware/ratelimit.go` — per-IP for `/auth/*`, per-APIKey for `/api/*`:
   ```go
   type perKeyLimiter struct{ mu sync.RWMutex; m map[string]*rate.Limiter; r, b int }
   func (p *perKeyLimiter) get(key string) *rate.Limiter {
       p.mu.RLock(); l := p.m[key]; p.mu.RUnlock()
       if l != nil { return l }
       p.mu.Lock(); defer p.mu.Unlock()
       if l := p.m[key]; l != nil { return l }
       l = rate.NewLimiter(rate.Limit(p.r), p.b); p.m[key] = l; return l
   }
   ```

7. **Guardrail**:

   `guardrail/secrets.go` — regex patterns per research §6.
   `guardrail/size.go`:
   ```go
   const MaxDiffBytes = 1 << 20 // 1 MiB
   const MaxChangedFiles = 200
   func CheckSize(diff *domain.Diff) error { ... }
   ```
   `guardrail/guardrail.go`:
   ```go
   type Result struct { Safe bool; Findings []Finding }
   type Finding struct { Kind string; Detail string; Blocking bool }
   func Check(diff *domain.Diff) Result { /* compose size + secrets */ }
   ```
   Decision: secrets → flag, don't block (research recommends Option B).

8. **Auth handlers** (`handlers/auth_handler.go`):
   - `POST /api/v1/auth/login` → verify bcrypt → sign JWT (access + refresh).
   - `POST /api/v1/auth/refresh` → verify refresh → re-sign access.
   - `GET /api/v1/me` → echo principal.

9. **Routes** (`routes.go`):
   ```go
   func Register(e *echo.Echo, h *Handlers, mw *Middleware) {
       e.GET("/healthz", h.Health.Get)
       public := e.Group("/api/v1/auth", mw.RateLimitIP)
       public.POST("/login", h.Auth.Login)
       public.POST("/refresh", h.Auth.Refresh)
       auth := e.Group("/api/v1", mw.JWT, mw.RateLimitUser)
       auth.GET("/me", h.Auth.Me)
       // Phase 06-12 attach more.
   }
   ```

10. **Wire in `cmd/server/main.go`**:
    ```go
    pool, _ := postgres.NewPool(ctx, cfg.DB)
    repos := postgres.NewRepos(pool)
    authUC := auth.NewUsecase(repos.User, repos.APIKey, cfg.Auth)
    handlers := httpdel.NewHandlers(authUC)
    mw := httpdel.NewMiddleware(cfg)
    e := echo.New()
    httpdel.Register(e, handlers, mw)
    ```

## Todo List

- [ ] domain entities compile with no repo imports
- [ ] pgxpool connects + retries on start
- [ ] bcrypt hash+verify unit-tested
- [ ] JWT sign+parse unit-tested
- [ ] API key: generate → hash → lookup → verify roundtrip
- [ ] Middleware chain attached
- [ ] `POST /auth/login` happy path returns JWT
- [ ] 401 on bad password
- [ ] Rate limiter returns 429 after burst
- [ ] Guardrail blocks oversize diff, flags secrets without block
- [ ] `GET /me` returns principal

## Success Criteria
- End-to-end: seed team+user → login → access `/me` with JWT → 200.
- Issue API key (manual SQL insert for now) → hit any `/api/v1/*` with `X-API-Key` → 200.
- Rate limit curl loop returns 429.
- `go test ./internal/guardrail/... ./internal/infrastructure/auth/...` passes.

## Risk Assessment
- **In-memory rate limiter + multi-instance**: fine for v1 (single-instance deploy). Phase 14 lists Redis migration path.
- **JWT rotation**: using `cfg.Auth.JWTSecret` static; rotation deferred.
- **bcrypt cost 12** ~250ms per hash; login p95 will exceed 300ms — acceptable for internal tool.

## Security Considerations
- Passwords: bcrypt cost 12.
- API keys never logged; mask in slog middleware.
- CORS: restrict to `web` origin from config.
- Timing-safe compare for API key hash (`subtle.ConstantTimeCompare`).
- Sensitive header redaction in logger middleware.

## Next Steps
- **Unblocks**: Phase 05 (review engine uses Principal + guardrail), Phase 06 (dashboard CRUD uses JWT), Phase 07/08 (webhook+REST use API key auth).
- **Parallel**: Phase 05, 06 within Group B.
