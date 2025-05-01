package ext_proc

import (
	ext_procv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"reflect"
	"testing"
)

func TestNewPromptGuard(t *testing.T) {
	tests := []struct {
		name string
		want *PromptGuard
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewPromptGuard(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewPromptGuard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPromptGuard_Process(t *testing.T) {
	type fields struct {
		apiKey      string
		baseURL     string
		fullBaseURL string
		modelName   string
		riskyToken  string
		client      *openai.Client
	}
	type args struct {
		srv ext_procv3.ExternalProcessor_ProcessServer
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := &PromptGuard{
				apiKey:      tt.fields.apiKey,
				baseURL:     tt.fields.baseURL,
				fullBaseURL: tt.fields.fullBaseURL,
				modelName:   tt.fields.modelName,
				riskyToken:  tt.fields.riskyToken,
				client:      tt.fields.client,
			}
			if err := pg.Process(tt.args.srv); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPromptGuard_checkRisk(t *testing.T) {
	type fields struct {
		apiKey      string
		baseURL     string
		fullBaseURL string
		modelName   string
		riskyToken  string
		client      *openai.Client
	}
	type args struct {
		userQuery string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := &PromptGuard{
				apiKey:      tt.fields.apiKey,
				baseURL:     tt.fields.baseURL,
				fullBaseURL: tt.fields.fullBaseURL,
				modelName:   tt.fields.modelName,
				riskyToken:  tt.fields.riskyToken,
				client:      tt.fields.client,
			}
			if got := pg.checkRisk(tt.args.userQuery); got != tt.want {
				t.Errorf("checkRisk() = %v, want %v", got, tt.want)
			}
		})
	}
}
