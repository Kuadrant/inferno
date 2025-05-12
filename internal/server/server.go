package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
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

	// Start the processor server
	if err := s.startProcessorServer(ctx); err != nil {
		log.Printf("Processor server error: %v", err)
		return err
	}

	return nil
}

func (s *Server) startProcessorServer(ctx context.Context) error {
	port := s.config.ExtProcPort
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	processor := ext_proc.NewProcessor()
	extProcPb.RegisterExternalProcessorServer(grpcServer, processor)
	grpc_health_v1.RegisterHealthServer(grpcServer, &HealthServer{})

	log.Printf("Ext_proc server listening on :%d", port)

	// Start server in a goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("Processor server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("Shutting down processor server")
	grpcServer.GracefulStop()
	return nil
}
