package testutil

import (
	"context"
	"fmt"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"io"
	"log"
	"sync"
)

// MockExtProcServer implements the ExternalProcessor_ProcessServer interface for testing.
type MockExtProcServer struct {
	grpc.ServerStream

	Requests      chan *extProcPb.ProcessingRequest
	Responses     chan *extProcPb.ProcessingResponse
	RecvErrs      chan error
	SendErrs      chan error
	Done          chan error
	SentResponses []*extProcPb.ProcessingResponse
	mu            sync.Mutex
	ctx           context.Context
	cancelCtx     context.CancelFunc
}

// NewMockExtProcServer creates an initialized mock server.
func NewMockExtProcServer(bufferSize int) *MockExtProcServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &MockExtProcServer{
		Requests:      make(chan *extProcPb.ProcessingRequest, bufferSize),
		Responses:     make(chan *extProcPb.ProcessingResponse, bufferSize),
		RecvErrs:      make(chan error, 1),
		SendErrs:      make(chan error, 1),
		Done:          make(chan error, 1),
		SentResponses: make([]*extProcPb.ProcessingResponse, 0),
		ctx:           ctx,
		cancelCtx:     cancel,
	}
}

func (m *MockExtProcServer) Send(resp *extProcPb.ProcessingResponse) error {
	select {
	case <-m.ctx.Done():
		log.Println("[MockServer] Send context done")
		return m.ctx.Err()
	default:
	}
	select {
	case err := <-m.SendErrs:
		log.Printf("[MockServer] Send returning injected error: %v", err)
		return err
	default:
	}

	log.Printf("[MockServer] Send called with: %+v", resp)
	m.mu.Lock()
	m.SentResponses = append(m.SentResponses, resp)
	m.mu.Unlock()

	select {
	case m.Responses <- resp:
		log.Printf("[MockServer] Response stored and signaled.")
	case <-m.ctx.Done():
		log.Println("[MockServer] Send context done before signaling response")
		return m.ctx.Err()
	default:
		log.Printf("[MockServer] Warning: Response channel full or not being read. Dropping signal for response: %+v", resp)
	}
	return nil
}

func (m *MockExtProcServer) Recv() (*extProcPb.ProcessingRequest, error) {
	select {
	case <-m.ctx.Done():
		log.Println("[MockServer] Recv context done")
		return nil, m.ctx.Err()
	case err := <-m.RecvErrs:
		log.Printf("[MockServer] Recv received error from RecvErrs chan: %v", err)
		return nil, err
	case req, ok := <-m.Requests:
		if !ok {
			log.Println("[MockServer] Recv read from closed Requests channel")
			return nil, io.EOF
		}
		log.Printf("[MockServer] Recv received request from Requests chan: %+v", req)
		if req == nil {
			log.Println("[MockServer] Warning: Received nil request from Requests channel")
			return nil, fmt.Errorf("[MockServer] received unexpected nil request")
		}
		return req, nil
	}
}

// InjectRequest sends a request into the mock server's Recv() stream.
func (m *MockExtProcServer) InjectRequest(req *extProcPb.ProcessingRequest) {
	if req == nil {
		log.Println("[MockServer] FATAL: InjectRequest called with nil request!")
		return
	}
	log.Printf("[MockServer] Injecting request: %+v", req)
	select {
	case m.Requests <- req:
	case <-m.ctx.Done():
		log.Println("[MockServer] InjectRequest context done, dropping request")
	default:
		log.Printf("[MockServer] Warning: Request channel full. Dropping request: %+v", req)
	}
}

// InjectRecvError causes the next call to Recv() to return the specified error.
func (m *MockExtProcServer) InjectRecvError(err error) {
	log.Printf("[MockServer] Injecting Recv error: %v", err)
	select {
	case m.RecvErrs <- err:
	case <-m.ctx.Done():
		log.Println("[MockServer] InjectRecvError context done, dropping error")
	default:
		log.Printf("[MockServer] Warning: RecvErrs channel full. Dropping error: %v", err)
	}
}

// InjectSendError causes the next call to Send() to return the specified error.
func (m *MockExtProcServer) InjectSendError(err error) {
	log.Printf("[MockServer] Injecting Send error: %v", err)
	select {
	case m.SendErrs <- err:
	case <-m.ctx.Done():
		log.Println("[MockServer] InjectSendError context done, dropping error")
	default:
		log.Printf("[MockServer] Warning: SendErrs channel full. Dropping error: %v", err)
	}
}

// Close cancels the context and cleans up the mock server's channels. Call this in AfterEach.
func (m *MockExtProcServer) Close() {
	log.Println("[MockServer] Closing (canceling context).")
	m.cancelCtx()

	safeClose := func(ch chan *extProcPb.ProcessingRequest) {
		defer func() {
			if recover() != nil {
			}
		}()
		close(ch)
	}
	safeCloseResponses := func(ch chan *extProcPb.ProcessingResponse) {
		defer func() {
			if recover() != nil {
			}
		}()
		close(ch)
	}
	safeCloseErr := func(ch chan error) {
		defer func() {
			if recover() != nil {
			}
		}()
		close(ch)
	}

	safeClose(m.Requests)
	safeCloseResponses(m.Responses)
	safeCloseErr(m.RecvErrs)
	safeCloseErr(m.SendErrs)
	log.Println("[MockServer] Channels closed.")
}
