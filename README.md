# Cursor OpenRouter Proxy

A high-performance HTTP/2-enabled proxy server that enables Cursor IDE (including Composer) to use any LLM available through OpenRouter. By making Cursor believe it's talking to GPT-4, this proxy translates OpenAI-compatible API requests to work with any model available on OpenRouter, allowing seamless integration with Cursor's features.

## Primary Use Case

This proxy enables Cursor IDE users to leverage any LLM available on OpenRouter through Cursor's interface, including the Composer. Simply point Cursor to this proxy with any key, and it will handle all the necessary translations to make your chosen model work as if it were GPT-4.

## Features

- Dynamic model switching via API endpoint without container reload
- HTTP/2 support for improved performance
- Full CORS support
- Streaming responses
- Support for function calling/tools
- Automatic message format conversion
- Compatible with OpenAI API client libraries
- API key validation
- Traefik integration ready
- Docker container support

## Prerequisites

- Cursor Pro Subscription
- OpenRouter API key
- Docker and Docker Compose
- Traefik (for reverse proxy)

## Quick Start with Docker Compose

1. Clone the repository
2. Configure environment:
   ```bash
   cp .env.example .env
   ```
   Edit `.env` and add your OpenRouter API key and preferred model

3. Start with Docker Compose:
   ```bash
   docker-compose up -d
   ```

## Configuration

The `.env` file controls your setup:

```bash
# Required
OPENROUTER_API_KEY=your_openrouter_api_key_here
# Optional - defaults to anthropic/claude-3-opus-20240229
OPENROUTER_MODEL=your_preferred_model
```

Available models can be found at [OpenRouter's model list](https://openrouter.ai/models).

## Usage

### Basic Usage

Point Cursor to `http://your-proxy:9000/v1` (or `https://cursor-proxy.$YOURDOMAIN/v1`) as the OpenAI API endpoint and keep GTP-4o as model.
The proxy will automatically:
1. Translate Cursor's GPT-4o requests to your chosen model
2. Handle all necessary format conversions
3. Stream responses back to Cursor

### Dynamic Model Switching

Switch models without restarting using the API endpoint:

```bash
curl -X POST http://your-proxy:9000/v1/config \
  -H "Content-Type: application/json" \
  -d '{"model": "anthropic/claude-3-opus-20240229"}'
```

### Traefik Integration

Update your docker-compose.yml to include Traefik labels:

```yaml
services:
  cursor-proxy:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.cursor-proxy.rule=Host(`your-domain`)"
      - "traefik.http.services.cursor-proxy.loadbalancer.server.port=9000"
```

## Supported Endpoints

- `/v1/chat/completions` - Chat completions endpoint
- `/v1/models` - Models listing endpoint
- `/switch-model` - Dynamic model switching endpoint

## Security

- CORS headers for cross-origin requests
- API key validation
- Secure request/response handling
- HTTPS support through HTTP/2
- Environment variables protection

## License

This project is licensed under the GNU General Public License v2.0 (GPLv2). See the [LICENSE.md](LICENSE.md) file for details.
