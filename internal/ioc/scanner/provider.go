package scanner

import (
	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
)

// InitProvider constructs the Solana HTTP reader for the scanner process.
func InitProvider(cfg config.RPCConfig) (provider.Reader, error) {
	return provider.NewReader(cfg.HTTPURL)
}
