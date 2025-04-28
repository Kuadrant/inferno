#!/bin/bash

# Build the application
echo "Building Inferno..."
go build -v

# Run the application
echo "Starting Inferno ext_proc service..."

# Check if running in K8s environment
if command -v kubectl &> /dev/null; then
  echo "Kubernetes environment detected, setting up connections to embedding model..."
  
  GATEWAY_HOST=$(kubectl get gateway -n kserve kserve-ingress-gateway -o jsonpath='{.status.addresses[0].value}')
  SERVICE_HOSTNAME=$(kubectl get inferenceservice embedding-model -o jsonpath='{.status.url}' | cut -d "/" -f 3)
  
  export EMBEDDING_MODEL_HOST="${EMBEDDING_MODEL_HOST:-$SERVICE_HOSTNAME}"
  export EMBEDDING_MODEL_SERVER="${EMBEDDING_MODEL_SERVER:-http://$GATEWAY_HOST/v1/models/embedding-model:predict}"
else
  # Default values for local development
  echo "Running in local environment..."
  export GUARDIAN_API_KEY=${GUARDIAN_API_KEY:-test}
  export GUARDIAN_URL=${GUARDIAN_URL:-http://example.com}
  export EMBEDDING_MODEL_SERVER=${EMBEDDING_MODEL_SERVER:-http://localhost:8081/v1/models/embedding-model:predict}
  export EMBEDDING_MODEL_HOST=${EMBEDDING_MODEL_HOST:-embedding-model.example.com}
fi

# Start the service
./inferno

# Example testing commands (commented out)
# 
# GATEWAY_HOST=$(kubectl get gateway -n kserve kserve-ingress-gateway -o jsonpath='{.status.addresses[0].value}')
# SERVICE_HOSTNAME=$(kubectl get inferenceservice huggingface-llm -o jsonpath='{.status.url}' | cut -d "/" -f 3)
# 
# echo "Testing LLM with semantic cache:"
# curl -v http://localhost:10000/semantic-cache/openai/v1/completions \
#   -H "content-type: application/json" \
#   -H "Host: $SERVICE_HOSTNAME" \
#   -d '{"model": "llm", "prompt": "What is Kubernetes", "stream": false, "max_tokens": 10}'
# 
# echo "Testing LLM with prompt guard:"
# curl -v http://localhost:10000/prompt-guard/openai/v1/completions \
#   -H "content-type: application/json" \
#   -H "Host: $SERVICE_HOSTNAME" \
#   -d '{"model": "llm", "prompt": "What is Docker", "stream": false, "max_tokens": 10}'