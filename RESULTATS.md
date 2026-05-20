# Results

## Context

- Project: `cursor-openrouter-proxy`
- Type: personal experiment / proof of concept
- Original goal: make Cursor call an OpenAI-compatible local endpoint while the proxy
  routed to OpenRouter.
- Main model tried in the article context: DeepSeek through OpenRouter.

## Tested Question

> Can a local OpenAI-compatible proxy let Cursor keep its normal API surface while
> another OpenRouter model answers behind it?

## Method

- Cursor was configured to call the proxy as an OpenAI-compatible endpoint.
- The proxy accepted `gpt-4o` from Cursor and rewrote it to the configured OpenRouter
  model.
- Requests and payload shape could be inspected at the proxy layer.

## Raw Results

| Check | Result | Comment |
| --- | --- | --- |
| Cursor request reached local proxy | passed | The proxy received OpenAI-style requests. |
| Proxy routed request to OpenRouter | passed | Model rewrite and forwarding worked. |
| Payload inspection | useful | Cursor prompts/options became observable. |
| DeepSeek answer usable as Cursor edit | inconsistent | Some responses stayed as chat text instead of becoming an applicable edit. |

## What This Shows

- The HTTP/API compatibility layer can be faked well enough to route requests.
- A proxy is useful for seeing what Cursor actually sends to a model.
- API compatibility is not enough when Cursor expects an implicit response format or
  behavior.

## What This Does Not Prove

- It does not prove that any OpenRouter model can replace Cursor's expected model.
- It does not prove compatibility with current Cursor versions.
- It does not prove safe handling for private code or sensitive prompts.
- It does not prove production readiness.

## Decision

- Keep the repo as an inspectable POC and payload-observation example.
- Do not present it as a robust Cursor model replacement without a fresh compatibility
  test.
