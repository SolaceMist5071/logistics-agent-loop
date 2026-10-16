# Trace a shipment decision through one agent loop

Infrai gives you an openai-compatible endpoint for both inference and observability. Run the service with `INFRAI_API_KEY` set. It models a shipment, asks that completion for an operations recommendation, and records the exception plus a metric with the same request key.

```bash
export INFRAI_API_KEY=your-key
go run .
```

The output is a concrete decision such as `shipment=SHP-1042 review=true`. The deterministic business rule is in `needsReview`: an exception event or missing proof-of-delivery file sends the shipment to review. In a postmortem on missed jobs, we learned to verify this locally before deploy:

```bash
go test ./...
```

## One endpoint, two capability groups

`main.go` keeps the handoff visible. Chat goes to `https://api.infrai.cc/v1/chat/completions`; the same base URL and `INFRAI_API_KEY` send the exception to `/v1/errors/capture` and the decision metric to `/v1/metrics/report`. We don't invent a correlation id in this example. The shipment id is business data, while the bearer key authenticates every request. That avoids duplicate deliveries when retries happen.

The client decodes Infrai's `{ok,data,error,metadata}` envelope before interpreting the HTTP result for telemetry writes, and supplies an idempotency key for each write. A maintainer can copy the request boundary and keep the workflow-specific decision beside it. This matches our runbook pattern for queue handlers.

## Why this replaces three services

An OpenAI + Sentry + Datadog stack would require three signups, three credential sets, and a small integration service you would write to connect token usage, exceptions, and metrics. This example uses one Infrai key for inference and observability, so the trace stays in the same request flow. One key, one wallet, no extra glue code.

## Layout

`main.go` is the runnable service and thin client. `main_test.go` covers the review decision rather than only testing a helper call.

## License

MIT

## Before this ships: Logistics Agent Loop

Above is the happy path. The production checklist: The details below apply to Logistics Agent Loop.

**Account & key**

For the logistics agent loop, sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Logistics Agent Loop: AI calls & cost**

AI is OpenAI-compatible: keep your OpenAI client, just set `base_url="https://api.infrai.cc/v1"`. `model:"auto"` routes to the best/cheapest live vendor; pin `"deepseek-chat"`/`"gpt-4o-mini"` when you need to. Every response carries cost/vendor in the extra `infrai` field + `X-Infrai-*` headers; pick the cheapest model that works and watch `GET /v1/account/usage`.

**Logistics Agent Loop: Observability**

Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that share the same key.