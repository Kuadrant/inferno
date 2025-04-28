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
# Build and run the Go service
go build
./inferno

# Or use docker-compose
docker-compose up --build
```

Later, we'll offer more options to deploy on Kubernetes, or as part of Kuadrant.

### Environment Variables

The following environment variables can be configured:

#### General Settings
- `SEMANTIC_CACHE_PORT`: Port for the semantic cache ext_proc (default: 50051)
- `PROMPT_GUARD_PORT`: Port for the prompt guard ext_proc (default: 50052)
- `TOKEN_METRICS_PORT`: Port for the token metrics ext_proc (default: 50053)

#### Semantic Cache Settings
- `EMBEDDING_MODEL_SERVER`: URL for the embedding model server
- `EMBEDDING_MODEL_HOST`: Host header for the embedding model server
- `SIMILARITY_THRESHOLD`: Threshold for semantic similarity (default: 0.75)

#### Prompt Guard Settings
- `GUARDIAN_API_KEY`: API key for the risk assessment model
- `GUARDIAN_URL`: Base URL for the risk assessment model
- `DISABLE_PROMPT_RISK_CHECK`: Set to "yes" to disable prompt risk checking
- `DISABLE_RESPONSE_RISK_CHECK`: Set to "yes" to disable response risk checking

## Sample Requests

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
