package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/kuadrant/inferno/internal/ext_proc"
)

type HealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

func (h *HealthServer) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (h *HealthServer) Watch(in *grpc_health_v1.HealthCheckRequest, srv grpc_health_v1.Health_WatchServer) error {
	return fmt.Errorf("watch is not implemented")
}

type Server struct {
	config *Config
}

func NewServer(cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Server{
		config: cfg,
	}
}

func (s *Server) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	// semantic cache server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.startSemanticCacheServer(ctx); err != nil {
			log.Printf("Semantic cache server error: %v", err)
		}
	}()

	// prompt guard server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.startPromptGuardServer(ctx); err != nil {
			log.Printf("Prompt guard server error: %v", err)
		}
	}()

	// token metrics server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.startTokenMetricsServer(ctx); err != nil {
			log.Printf("Token metrics server error: %v", err)
		}
	}()

	// wait for all servers to exit
	wg.Wait()
	return nil
}

func (s *Server) startSemanticCacheServer(ctx context.Context) error {
	port := s.config.SemanticCachePort
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	semanticCache := ext_proc.NewSemanticCache()
	extProcPb.RegisterExternalProcessorServer(grpcServer, semanticCache)
	grpc_health_v1.RegisterHealthServer(grpcServer, &HealthServer{})

	log.Printf("Semantic cache ext_proc server listening on :%d", port)

	// start
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("Semantic cache server error: %v", err)
		}
	}()

	// wait for cancel
	<-ctx.Done()
	log.Println("Shutting down semantic cache server")
	grpcServer.GracefulStop()
	return nil
}

func (s *Server) startPromptGuardServer(ctx context.Context) error {
	port := s.config.PromptGuardPort
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	promptGuard := ext_proc.NewPromptGuard()
	extProcPb.RegisterExternalProcessorServer(grpcServer, promptGuard)
	grpc_health_v1.RegisterHealthServer(grpcServer, &HealthServer{})

	log.Printf("Prompt guard ext_proc server listening on :%d", port)

	// start server
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("Prompt guard server error: %v", err)
		}
	}()

	// wait
	<-ctx.Done()
	log.Println("Shutting down prompt guard server")
	grpcServer.GracefulStop()
	return nil
}

func (s *Server) startTokenMetricsServer(ctx context.Context) error {
	port := s.config.TokenMetricsPort
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	tokenMetrics := ext_proc.NewTokenUsageMetrics()
	extProcPb.RegisterExternalProcessorServer(grpcServer, tokenMetrics)
	grpc_health_v1.RegisterHealthServer(grpcServer, &HealthServer{})

	log.Printf("Token metrics ext_proc server listening on :%d", port)

	// start server
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("Token metrics server error: %v", err)
		}
	}()

	// wait
	<-ctx.Done()
	log.Println("Shutting down token metrics server")
	grpcServer.GracefulStop()
	return nil
}
