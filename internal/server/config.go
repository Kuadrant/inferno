package server

type Config struct {
	// Port configurations for each ext_proc
	SemanticCachePort  int
	PromptGuardPort    int
	TokenMetricsPort   int
}

func DefaultConfig() *Config {
	return &Config{
		SemanticCachePort:  50051,
		PromptGuardPort:    50052,
		TokenMetricsPort:   50053,
	}
}