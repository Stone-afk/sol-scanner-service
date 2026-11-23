package config

import "time"

// Config is the normalized configuration for the scanner process.
type Config struct {
	Chain    ChainConfig
	RPC      RPCConfig
	Scanner  ScannerConfig
	Database DatabaseConfig
	Kafka    KafkaConfig
}

// ProcessorConfig is the normalized configuration for the processor process.
// It has no RPC endpoint and cannot be used to construct an RPC client.
type ProcessorConfig struct {
	Chain        ChainConfig
	Scanner      ScannerConfig
	Database     DatabaseConfig
	Kafka        KafkaConfig
	ParseWorkers int // processor.parseWorkers; default 4, range 1–64
}

// ChainConfig holds chain identity and the optional start slot.
type ChainConfig struct {
	Name        string
	GenesisHash string
	StartSlot   uint64 // 0 = unset
}

// RPCConfig holds the HTTP RPC endpoint used by the scanner process.
type RPCConfig struct {
	HTTPURL string
}

// ScannerConfig holds polling and step-size parameters.
type ScannerConfig struct {
	PollInterval   time.Duration
	StepSlots      uint64
	SlotsPerWorker uint64
}

// DatabaseConfig holds the database connection configuration.
type DatabaseConfig struct {
	DSN string
}

// KafkaConfig holds Kafka broker and topic settings.
type KafkaConfig struct {
	Brokers          []string
	BlockTopic       string
	TransactionTopic string
}

type rawConfig struct {
	Chain     rawChainConfig    `yaml:"chain"`
	RPC       rawRPCConfig      `yaml:"rpc"`
	Scanner   rawScannerConfig  `yaml:"scanner"`
	Database  rawDatabaseConfig `yaml:"database"`
	Kafka     rawKafkaConfig    `yaml:"kafka"`
	Processor any               `yaml:"processor"` // LoadForScanner ignores
}

// rawProcessorConfig accepts every target key. rpc uses the same fixed
// shape as the scanner so unknown keys fail, but its URL is not expanded
// or copied into ProcessorConfig.
type rawProcessorConfig struct {
	Chain     rawChainConfig      `yaml:"chain"`
	RPC       rawRPCConfig        `yaml:"rpc"`
	Scanner   rawScannerConfig    `yaml:"scanner"`
	Database  rawDatabaseConfig   `yaml:"database"`
	Kafka     rawKafkaConfig      `yaml:"kafka"`
	Processor rawProcessorSection `yaml:"processor"`
}

type rawMigrationConfig struct {
	Database  rawDatabaseConfig `yaml:"database"`
	Chain     any               `yaml:"chain"`
	RPC       any               `yaml:"rpc"`
	Scanner   any               `yaml:"scanner"`
	Kafka     any               `yaml:"kafka"`
	Processor any               `yaml:"processor"`
}

// rawProcessorSection holds processor-only knobs. parseWorkers is any so
// non-integers fail with a field-path error instead of a generic YAML type error.
type rawProcessorSection struct {
	ParseWorkers any `yaml:"parseWorkers"`
}

type rawChainConfig struct {
	Name        string `yaml:"name"`
	GenesisHash string `yaml:"genesisHash"`
	// Nil means the key was omitted. Both omission and 0 normalize to StartSlot 0.
	StartSlot *uint64 `yaml:"startSlot"`
}

type rawRPCConfig struct {
	HTTPURL string `yaml:"httpURL"`
}

type rawScannerConfig struct {
	PollInterval   string `yaml:"pollInterval"`
	StepSlots      uint64 `yaml:"stepSlots"`
	SlotsPerWorker uint64 `yaml:"slotsPerWorker"`
}

type rawDatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type rawKafkaConfig struct {
	Brokers          []string `yaml:"brokers"`
	BlockTopic       string   `yaml:"blockTopic"`
	TransactionTopic string   `yaml:"transactionTopic"`
}
