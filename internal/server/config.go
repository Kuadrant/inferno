package server

type Config struct {
	ExtProcPort int
}

func DefaultConfig() *Config {
	return &Config{
		ExtProcPort: 50051,
	}
}
