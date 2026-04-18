# Phase 09 — RabbitMQ Job Queue

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-01-backend-report.md` §4 (RabbitMQ patterns, DLQ, reconnect)
- Depends on: Phase 04 (domain + config), Phase 05 (review engine)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 8h
- **Group**: C (parallel with 07, 08)
- **Description**: Durable async job queue with `code-review-jobs` main queue, `code-review-jobs-dlq` dead-letter, publisher confirms, consumer QoS, app-level reconnect. Producer used by Phase 07/08; consumer runs in `cmd/worker`.

## Key Insights
- Research §4: amqp091-go does NOT auto-reconnect; wrap in supervised loop that re-declares topology after reconnect.
- Publisher confirms (`ch.Confirm(false)`) for durable publish; wait for `NotifyPublish` channel.
- Consumer QoS prefetch = N workers × 2.
- Max delivery attempts via message header `x-delivery-count`; after 3 → route to DLQ.
- Job payload: `ReviewJob{ID, TeamID, Source, DiffContent?, RepoFullName?, PRNumber?, HeadSHA?, BaseSHA?, IntegrationID?}`. For webhook source, DiffContent is empty → worker fetches via integration API.

## Requirements

### Functional
- `queue.Publisher` interface with `Publish(ctx, job)`.
- `queue.Consumer` with `Start(ctx, handler)`.
- Topology declaration idempotent on startup.
- Reconnect with exponential backoff (1s → 30s cap).
- Dead-letter routing after 3 failed deliveries.
- Worker binary `cmd/worker` wires consumer → `review.Usecase.Run` → `webhook.Poster.Post` (if source is github/gitlab).

### Non-functional
- Message durability: `DeliveryMode: Persistent` + durable queue.
- Consumer parallelism: configurable, default 4 goroutines per worker.
- Graceful shutdown drains in-flight tasks up to 30s.

## Architecture

```
internal/
├── infrastructure/queue/
│   ├── connection.go           # managed conn/channel with auto-reconnect
│   ├── topology.go             # declare queues/exchanges
│   ├── publisher.go            # confirmed publish
│   ├── consumer.go             # QoS + dispatch
│   └── types.go                # ReviewJob + envelope
└── usecase/worker/
    ├── handler.go              # Handle(ctx, job) -> error
    └── run.go                  # wiring: bootstrap consumer + handler
cmd/
└── worker/main.go              # entrypoint (flesh out Phase 01 stub)
```

## Related Code Files

### Create
- `/internal/infrastructure/queue/{connection,topology,publisher,consumer,types}.go`
- `/internal/usecase/worker/{handler,run}.go`

### Modify
- `/cmd/worker/main.go` — wire config → deps → worker.Run.
- `/internal/config/config.go` — `AMQP{Prefetch int; Parallelism int; ReconnectBackoffMax time.Duration}`.

## Implementation Steps

1. **Envelope + job type** (`queue/types.go`):
   ```go
   type ReviewJob struct {
       JobID uuid.UUID
       TeamID uuid.UUID
       Source string                // github/gitlab/api-async (future)
       DiffContent string           // present for direct; empty for webhook → fetch
       RepoFullName, HeadSHA, BaseSHA string
       PRNumber int
       IntegrationID uuid.UUID
       Attempt int
   }
   const (QueueReviewJobs="code-review-jobs"; QueueReviewDLQ="code-review-jobs-dlq"; MaxAttempts=3)
   ```

2. **Managed connection** (`connection.go`, <150 lines):
   ```go
   type Managed struct {
       url string; mu sync.RWMutex
       conn *amqp.Connection; ch *amqp.Channel
       closeSignal chan struct{}
       log *slog.Logger
   }
   func (m *Managed) Channel() *amqp.Channel { m.mu.RLock(); defer m.mu.RUnlock(); return m.ch }
   func (m *Managed) Run(ctx context.Context) error {
       backoff := time.Second
       for {
           if err := m.connect(); err != nil {
               select { case <-ctx.Done(): return ctx.Err(); case <-time.After(backoff): }
               if backoff < 30*time.Second { backoff *= 2 }; continue
           }
           backoff = time.Second
           notify := m.conn.NotifyClose(make(chan *amqp.Error, 1))
           select { case <-ctx.Done(): m.conn.Close(); return ctx.Err(); case err := <-notify: m.log.Warn("amqp closed", "err", err) }
       }
   }
   func (m *Managed) connect() error {
       conn, err := amqp.Dial(m.url); if err != nil { return err }
       ch, err := conn.Channel(); if err != nil { conn.Close(); return err }
       if err := DeclareTopology(ch); err != nil { conn.Close(); return err }
       m.mu.Lock(); m.conn, m.ch = conn, ch; m.mu.Unlock()
       return nil
   }
   ```

3. **Topology** (`topology.go`):
   ```go
   func DeclareTopology(ch *amqp.Channel) error {
       if _, err := ch.QueueDeclare(QueueReviewDLQ, true, false, false, false, nil); err != nil { return err }
       _, err := ch.QueueDeclare(QueueReviewJobs, true, false, false, false, amqp.Table{
           "x-dead-letter-exchange":    "",
           "x-dead-letter-routing-key": QueueReviewDLQ,
       })
       return err
   }
   ```

4. **Publisher** (`publisher.go`):
   ```go
   type Publisher struct { m *Managed }
   func (p *Publisher) Publish(ctx context.Context, job ReviewJob) error {
       ch := p.m.Channel(); if ch == nil { return ErrNotReady }
       if err := ch.Confirm(false); err != nil { return err }
       confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
       body, _ := json.Marshal(job)
       if err := ch.PublishWithContext(ctx, "", QueueReviewJobs, false, false, amqp.Publishing{
           ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: job.JobID.String(), Body: body,
       }); err != nil { return err }
       select {
       case c := <-confirms: if !c.Ack { return ErrNack }; return nil
       case <-time.After(5*time.Second): return ErrConfirmTimeout
       case <-ctx.Done(): return ctx.Err()
       }
   }
   ```

5. **Consumer** (`consumer.go`):
   ```go
   type Consumer struct { m *Managed; parallelism int; prefetch int; log *slog.Logger }
   type Handler func(ctx context.Context, job ReviewJob) error
   func (c *Consumer) Start(ctx context.Context, h Handler) error {
       ch := c.m.Channel(); if err := ch.Qos(c.prefetch, 0, false); err != nil { return err }
       del, err := ch.Consume(QueueReviewJobs, "", false, false, false, false, nil)
       if err != nil { return err }
       sem := make(chan struct{}, c.parallelism)
       for {
           select {
           case <-ctx.Done(): return ctx.Err()
           case d, ok := <-del:
               if !ok { return errors.New("deliveries closed") }
               sem <- struct{}{}
               go func(d amqp.Delivery) {
                   defer func(){ <-sem }()
                   var job ReviewJob; if err := json.Unmarshal(d.Body, &job); err != nil { d.Nack(false, false); return }
                   hctx, cancel := context.WithTimeout(ctx, 2*time.Minute); defer cancel()
                   if err := h(hctx, job); err != nil {
                       attempts := getDeathCount(d.Headers) + 1
                       if attempts >= MaxAttempts { d.Nack(false, false); return }   // DLQ
                       d.Nack(false, true); return                                    // requeue
                   }
                   d.Ack(false)
               }(d)
           }
       }
   }
   ```

6. **Worker handler** (`usecase/worker/handler.go`):
   ```go
   type Handler struct {
       ReviewUC *review.Usecase
       Poster   *webhook.Poster
       Integ    domain.IntegrationRepo
       GH       github.API
       GL       gitlab.API
   }
   func (h *Handler) Handle(ctx context.Context, job ReviewJob) error {
       diff := job.DiffContent
       if diff == "" && job.IntegrationID != uuid.Nil {
           integ, err := h.Integ.Get(ctx, job.IntegrationID); if err != nil { return err }
           switch job.Source {
           case "github": diff, err = h.GH.GetPRDiff(ctx, job.RepoFullName, job.PRNumber, integ.APIToken)
           case "gitlab": diff, err = h.GL.GetMRDiff(ctx, job.RepoFullName, job.PRNumber, integ.APIToken)
           }
           if err != nil { return err }
       }
       res, err := h.ReviewUC.Run(ctx, review.Request{ TeamID: job.TeamID, Source: job.Source, DiffContent: diff, RepoFullName: job.RepoFullName, HeadSHA: job.HeadSHA, BaseSHA: job.BaseSHA })
       if err != nil { return err }
       if job.Source == "github" || job.Source == "gitlab" {
           if err := h.Poster.Post(ctx, *res.Review, res.Issues); err != nil { return err }
       }
       return nil
   }
   ```

7. **Worker entrypoint** (`cmd/worker/main.go`, <80 lines):
   ```go
   func main() {
       cfg := config.Load()
       pool, _ := postgres.NewPool(ctx, cfg.DB)
       deps := bootstrap.NewWorker(cfg, pool)
       mgr := queue.NewManaged(cfg.AMQP.URL)
       go mgr.Run(ctx)
       <-mgr.Ready()
       consumer := queue.NewConsumer(mgr, cfg.AMQP.Parallelism, cfg.AMQP.Prefetch)
       if err := consumer.Start(ctx, deps.Handler.Handle); err != nil { log.Error(err) }
       <-ctx.Done()
   }
   ```

8. **Integrate publisher in Phase 07 webhook handler** — replace fallback with real publisher.

## Todo List

- [ ] Topology declared idempotently (run twice → no error)
- [ ] Publisher confirms `Ack=true` on normal publish
- [ ] Consumer QoS limits in-flight messages
- [ ] Kill rabbit → restart → managed connection reconnects, resumes consumption
- [ ] Force handler error 3× → message lands in `code-review-jobs-dlq`
- [ ] Graceful shutdown drains in-flight (SIGTERM → wait for in-flight completion)
- [ ] Publish 100 jobs; all processed within expected time

## Success Criteria
- Integration test: publish → worker handles → `reviews` row exists.
- Stop rabbitmq docker, publish (expect fail), start rabbit → next publish succeeds.
- `x-death`/attempt count triggers DLQ correctly.

## Risk Assessment
- **Channel per producer**: single publisher channel serializes publishes → bottleneck at high volume. v1 OK; document upgrade to channel pool.
- **Message poison loop**: MaxAttempts=3 + DLQ guards against infinite retries.
- **Worker crash mid-process**: AMQP redelivery + idempotent review persist (upsert on `diff_hash+team_id`? No — allow multiple rows, mark latest; cheaper).

## Security Considerations
- AMQP credentials from env only.
- Payloads contain diff content — enable RabbitMQ TLS in production (Phase 14).
- No PII; logs show `job_id` only.

## Next Steps
- **Unblocks**: Phase 10 (async embedding regeneration via queue possible), Phase 11 (queue-driven slack notify), Phase 13 (queue in integration harness).
- **Parallel**: Phase 07, 08.
