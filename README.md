# Inferno

A service providing a suite of `ext_proc` services for LLM use-cases:

- **Semantic Cache**: Caches responses based on semantic similarity of prompts
- **Prompt Guard**: Filters and blocks potentially harmful prompts using LLM-based risk detection
- **Token Usage Metrics**: Parses token usage for monitoring and rate limiting use-cases

## Running Locally

### Prerequisites

- Go 1.23+
- Docker / Podman / Kubernetes

### Building and Running

Currently, we offer a way to run a demo version of the service, alongside a pre-configured Envoy instance.

```bash
# Builds `inferno` and deploys Envoy & configures it to use inferno filter
docker-compose up --build
```

Later, we'll offer more options to deploy on Kubernetes, or as part of Kuadrant.

### Environment Variables

The following environment variables can be configured:

#### General Settings
- `EXT_PROC_PORT`: Port for the ext_proc server (default: 50051)

#### Semantic Cache Settings
- `EMBEDDING_MODEL_SERVER`: URL for the embedding model server
- `EMBEDDING_MODEL_HOST`: Host header for the embedding model server
- `SIMILARITY_THRESHOLD`: Threshold for semantic similarity (default: 0.75)

#### Prompt Guard Settings
- `GUARDIAN_API_KEY`: API key for the risk assessment model
- `GUARDIAN_URL`: Base URL for the risk assessment model
- `DISABLE_PROMPT_RISK_CHECK`: Set to "yes" to disable prompt risk checking
- `DISABLE_RESPONSE_RISK_CHECK`: Set to "yes" to disable response risk checking

#### API Endpoint Settings
- `OPENAI_API_HOST`: Hostname for OpenAI API requests (default: api.openai.com)
- `KSERVE_API_HOST`: Hostname/IP for KServe API requests (default: 192.168.97.4)
- `KSERVE_API_HOST_HEADER`: Host header value for KServe API requests (default: huggingface-llm-default.example.com)

## Sample Requests

### OpenAI proxied requests

The demo setup with `docker compose` configures Envoy to proxy chat completion and embeddings requests to OpenAI's API, as well as our sample filter chain with the `ext_proc` services we provision and run. Ensure you have a valid OpenAI API key exported as an environment variable:

```bash
export OPENAI_API_KEY=xxx
```


#### Completion

```bash
curl "http://localhost:10000/v1/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -d '{
      "model": "gpt-3.5-turbo-instruct",
      "prompt": "Write a one-sentence bedtime story about Kubernetes."
  }'
```

#### Chat completion

Chat completions:

```bash
curl -v "http://localhost:10000/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -d '{
      "model": "gpt-4.1",
      "messages": [
        {
          "role": "user",
          "content": "Write a one-sentence bedtime story about Kubernetes."
        }
      ]
  }'
```


Responses:

```bash
curl -v http://localhost:10000/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -d '{
    "model": "gpt-4.1",
    "input": "Tell me a three sentence bedtime story about Kubernetes."
  }'
```

#### Embeddings

```bash
curl http://localhost:10000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -d '{
    "input": "Your text string goes here",
    "model": "text-embedding-3-small"
  }'
```

### KServe Hugging Face LLM Runtime

Inferno supports KServe's Hugging Face LLM runtime API endpoints. These endpoints use the `/openai/v1/` prefix instead of `/v1/`. You can configure the KServe host using environment variables.

#### Configuration

You can set the following environment variables to configure the KServe integration, if running embedding and LLM models as inference services:

```bash
# Set the KServe destination address/IP (default: 192.168.97.4)
export KSERVE_API_HOST=192.168.97.4

# Set the KServe Host header separately (default: huggingface-llm-default.example.com)
export KSERVE_API_HOST_HEADER=huggingface-llm-default.example.com

export EMBEDDING_MODEL_SERVER=http://192.168.97.4/v1/models/embedding-model:predict

# Optional: Set the KServe Host header (if different, otherwise don't export/leave blank)
# export EMBEDDING_MODEL_HOST="embedding-model-default.example.com"


# or set these dynamically, for example:
export KSERVE_API_HOST="$(kubectl get gateway -n kserve kserve-ingress-gateway -o jsonpath='{.status.addresses[0].value}')"
export KSERVE_API_HOST_HEADER="$(kubectl get inferenceservice huggingface-llm -o jsonpath='{.status.url}' | cut -d '/' -f 3)"
export EMBEDDING_MODEL_SERVER="http://$(kubectl get gateway -n kserve kserve-ingress-gateway -o jsonpath='{.status.addresses[0].value}')/v1/models/embedding-model:predict"
export EMBEDDING_MODEL_HOST="$(kubectl get inferenceservice embedding-model -o jsonpath='{.status.url}' | cut -d '/' -f 3)"


# Start Inferno with the KServe configuration
docker-compose up --build
```

**Note:** KServe's Huggingface LLM runtime expects requests at `/openai/v1/...` paths, not `/v1/...` paths - `inferno` preserves these paths and does not rewrite them.

With this configuration, you can make simplified requests to your local Inferno instance:

```bash
# Without needing to specify the Host header in each request
curl -v http://localhost:10000/openai/v1/completions \
  -H "content-type: application/json" \
  -d '{"model": "llm", "prompt": "What is Kubernetes", "stream": false, "max_tokens": 50}'
```

#### Completions

```bash
curl -v http://localhost:10000/openai/v1/completions \
  -H "content-type: application/json" \
  -d '{"model": "llm", "prompt": "What is Kubernetes", "stream": false, "max_tokens": 50}'
```

#### Chat Completions

```bash
curl -v "http://localhost:10000/openai/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
      "model": "llm",
      "messages": [
        {
          "role": "system",
          "content": "You are an assistant that knows everything about Kubernetes."
        },
        {
          "role": "user",
          "content": "What is Kubernetes"
        }
      ],
      "max_tokens": 30,
      "stream": false
  }'
```

The responses from the KServe Hugginfface LLM server follow the OpenAI-style APIs, and include token usage metrics that Inferno will extract and add as headers in responses.

### Semantic Cache

```bash
curl -v 
```

### Prompt Guard

```bash
curl -v 
```

### Token Usage Metrics

```bash
curl -v 
```

## Testing

To run the unit tests locally, use the following command:

```bash
make test
````
**Note:** The tests are only starting to be written, and are not comprehensive yet. We will be adding more tests in the future.


  # TODO completions vs chat-completions

