#!/bin/sh
set -e

# Default values
OPENAI_API_HOST=${OPENAI_API_HOST:-api.openai.com}
KSERVE_API_HOST=${KSERVE_API_HOST:-192.168.97.4}
KSERVE_API_HOST_HEADER=${KSERVE_API_HOST_HEADER:-huggingface-llm-default.example.com}

# Make sure the directory exists
mkdir -p /etc/envoy

# Create a templated config file
cat > /etc/envoy/envoy.yaml << EOF
admin:
  access_log_path: /tmp/admin_access.log
  address:
    socket_address:
      protocol: TCP
      address: 0.0.0.0
      port_value: 15000

static_resources:
  listeners:
    - name: listener_http
      address:
        socket_address:
          protocol: TCP
          address: 0.0.0.0
          port_value: 10000
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                generate_request_id: true
                codec_type: AUTO
                route_config:
                  name: local_routes
                  virtual_hosts:
                    - name: api_proxy
                      domains: ["*"]
                      routes:
                        # Standard OpenAI API endpoints
                        - match:
                            prefix: "/v1/completions"
                          route:
                            cluster: openai_api
                            timeout: 30s
                            host_rewrite_literal: "${OPENAI_API_HOST}"
                        - match:
                            prefix: "/v1/chat/completions"
                          route:
                            cluster: openai_api
                            timeout: 30s
                            host_rewrite_literal: "${OPENAI_API_HOST}"
                        - match:
                            prefix: "/v1/responses"
                          route:
                            cluster: openai_api
                            timeout: 30s
                            host_rewrite_literal: "${OPENAI_API_HOST}"
                        - match:
                            prefix: "/v1/embeddings"
                          route:
                            cluster: openai_api
                            timeout: 30s
                            host_rewrite_literal: "${OPENAI_API_HOST}"
                        
                        # KServe Hugging Face LLM runtime endpoints
                        - match:
                            prefix: "/openai/v1/completions"
                          route:
                            cluster: kserve_api
                            timeout: 30s
                            host_rewrite_literal: "${KSERVE_API_HOST_HEADER}"
                        - match:
                            prefix: "/openai/v1/chat/completions"
                          route:
                            cluster: kserve_api
                            timeout: 30s
                            host_rewrite_literal: "${KSERVE_API_HOST_HEADER}"
                http_filters:
                  - name: envoy.filters.http.ext_proc
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor
                      failure_mode_allow: false
                      message_timeout: 30s
                      processing_mode:
                        request_header_mode: SEND
                        request_body_mode: BUFFERED
                        response_header_mode: SEND
                        response_body_mode: BUFFERED
                      grpc_service:
                        google_grpc:
                          target_uri: inferno_ext_proc:50051
                          stat_prefix: ext-proc
                        timeout: 30s
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: openai_api
      connect_timeout: 5s
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: openai_api
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: ${OPENAI_API_HOST}
                      port_value: 443
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          sni: ${OPENAI_API_HOST}
    
    - name: kserve_api
      connect_timeout: 5s
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: kserve_api
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: ${KSERVE_API_HOST}
                      port_value: 80
EOF

# Execute Envoy with the generated config
exec /usr/local/bin/envoy --config-path /etc/envoy/envoy.yaml "$@"