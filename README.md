# Cursor OpenRouter Proxy

This repo is an old experiment: a small Go proxy that lets Cursor call a local
OpenAI-compatible endpoint while the proxy forwards the request to OpenRouter.

It is useful for inspecting the shape of Cursor requests and testing model routing. It
is not a maintained replacement for Cursor's native model integrations.

## What It Is For

- Point Cursor at a local `/v1` endpoint.
- Rewrite Cursor's `gpt-4o` request to the configured OpenRouter model.
- Observe and debug the payloads Cursor sends.
- Test whether another model can behave well enough inside Cursor's workflow.

## What You Get

- A minimal Go proxy.
- Docker and Docker Compose files.
- Runtime model switching through `/v1/config`.
- A small test script for `/v1/chat/completions`.

## Read Before Running

- Main risk: Cursor payloads and prompts pass through this proxy. Do not use it with
  private code or sensitive prompts unless you are comfortable with that path.
- Prerequisites: Cursor, an OpenRouter API key, Docker, and Docker Compose.
- This is an experiment, not a robust product.
- The original Cursor/DeepSeek test showed a limit: the HTTP request could route
  successfully, but Cursor did not always turn the model response into an applicable
  code edit.
- The Traefik Compose file is optional. Use the local Compose file first unless you
  already have the external `proxy` network and domain setup.

## Quick Local Run

```bash
cp .env.example .env
docker compose -f docker-compose.local.yml up -d --build
PROXY_URL=http://127.0.0.1:9000 ./test_proxy.sh
```

Point Cursor to:

```txt
http://127.0.0.1:9000/v1
```

Keep `gpt-4o` as the model in Cursor. The proxy rewrites that model to the
`OPENROUTER_MODEL` configured in `.env`.

## Configuration

```bash
OPENROUTER_API_KEY=your_openrouter_api_key_here
OPENROUTER_MODEL=deepseek/deepseek-chat
```

Available models are listed by OpenRouter: <https://openrouter.ai/models>.

## Useful Endpoints

| Endpoint | Usage |
| --- | --- |
| `/v1/chat/completions` | OpenAI-compatible chat endpoint used by Cursor |
| `/v1/models` | Model listing endpoint |
| `/v1/config` | `GET` current config or `POST {"model":"..."}` to switch model |
| `/health` | Local health check |

Example model switch:

```bash
curl -X POST http://127.0.0.1:9000/v1/config \
  -H "Content-Type: application/json" \
  -d '{"model": "deepseek/deepseek-chat"}'
```

## Traefik Path

The default `docker-compose.yml` assumes an existing external Docker network named
`proxy` and a domain configured for Traefik. Use it only if that matches your machine.

```bash
docker compose up -d --build
```

Cursor endpoint example:

```txt
https://cursor-proxy.example.com/v1
```

## Safety Notes

- The proxy uses your real OpenRouter API key server-side.
- Cursor can send large prompts, tool descriptions, and file context.
- Do not assume every OpenRouter model will follow Cursor's implicit edit format.
- Do not expose the proxy publicly without an access layer you understand.

## Limitations

- Tested as a personal experiment, not as a maintained integration.
- No broad compatibility matrix across Cursor versions or OpenRouter models.
- No guarantee that streamed responses, tool calls, or edit application semantics match
  what Cursor expects today.
- See [`RESULTATS.md`](./RESULTATS.md) for the factual result summary and caveats.

## Related Article

Project Pezzos keeps the context for this experiment in
[Cursor OpenRouter Proxy](https://projectpezzoscom.pages.dev/journal/cursor-openrouter-proxy/).
The article explains why this proxy was built, what it showed about Cursor payloads, and
where the Cursor/DeepSeek integration stayed fragile.

## License

This project is licensed under the GNU General Public License v2.0 (GPLv2). See
[`LICENSE.md`](./LICENSE.md).
