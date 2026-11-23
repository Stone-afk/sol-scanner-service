package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var environmentVariable = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
var residualEnvironmentVariable = regexp.MustCompile(`\$\{[^}]*\}`)
var unknownField = regexp.MustCompile(`field ([A-Za-z0-9_]+) not found`)

// LoadForScanner loads and validates configuration for the scanner process.
// transactionTopic may be empty. A non-empty value must match the topic rule.
func LoadForScanner(path string, lookupEnv func(string) (string, bool)) (Config, error) {
	raw, err := loadRaw(path)
	if err != nil {
		return Config{}, err
	}

	httpURL, err := expandEnvironment(raw.RPC.HTTPURL, lookupEnv)
	if err != nil {
		return Config{}, err
	}
	dsn, err := expandEnvironment(raw.Database.DSN, lookupEnv)
	if err != nil {
		return Config{}, err
	}
	brokers, err := expandBrokers(raw.Kafka.Brokers, lookupEnv)
	if err != nil {
		return Config{}, err
	}

	cfg, err := normalizeScanner(raw, httpURL, dsn, brokers)
	if err != nil {
		return Config{}, err
	}
	if err := validateScanner(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadForProcessor loads and validates configuration for the processor process.
// It does not expand rpc.httpURL and returns no RPC client settings.
// transactionTopic is required.
func LoadForProcessor(path string, lookupEnv func(string) (string, bool)) (ProcessorConfig, error) {
	raw, err := loadRawProcessor(path)
	if err != nil {
		return ProcessorConfig{}, err
	}

	dsn, err := expandEnvironment(raw.Database.DSN, lookupEnv)
	if err != nil {
		return ProcessorConfig{}, err
	}
	brokers, err := expandBrokers(raw.Kafka.Brokers, lookupEnv)
	if err != nil {
		return ProcessorConfig{}, err
	}

	cfg, err := normalizeProcessor(raw, dsn, brokers)
	if err != nil {
		return ProcessorConfig{}, err
	}
	if err := validateProcessor(cfg); err != nil {
		return ProcessorConfig{}, err
	}
	return cfg, nil
}

// LoadForMigration loads only the database configuration required by migration.
func LoadForMigration(path string, lookupEnv func(string) (string, bool)) (DatabaseConfig, error) {
	raw, err := loadRawMigration(path)
	if err != nil {
		return DatabaseConfig{}, err
	}

	dsn, err := expandEnvironment(raw.Database.DSN, lookupEnv)
	if err != nil {
		return DatabaseConfig{}, err
	}
	database := DatabaseConfig{DSN: dsn}
	if err := validateDatabase(database); err != nil {
		return DatabaseConfig{}, err
	}
	return database, nil
}

func loadRaw(path string) (rawConfig, error) {
	var raw rawConfig
	if err := decodeYAML(path, &raw); err != nil {
		return rawConfig{}, err
	}
	return raw, nil
}

func loadRawProcessor(path string) (rawProcessorConfig, error) {
	var raw rawProcessorConfig
	if err := decodeYAML(path, &raw); err != nil {
		return rawProcessorConfig{}, err
	}
	return raw, nil
}

func loadRawMigration(path string) (rawMigrationConfig, error) {
	var raw rawMigrationConfig
	if err := decodeYAML(path, &raw); err != nil {
		return rawMigrationConfig{}, err
	}
	return raw, nil
}

func decodeYAML(path string, dest any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read configuration file: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(dest); err != nil {
		return invalidYAML(err)
	}
	return rejectTrailingYAMLDocument(decoder)
}

func rejectTrailingYAMLDocument(decoder *yaml.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("invalid YAML configuration")
	}
	return nil
}

func invalidYAML(err error) error {
	msg := err.Error()
	match := unknownField.FindStringSubmatch(msg)
	if len(match) != 2 {
		return fmt.Errorf("invalid YAML configuration")
	}
	field := match[1]
	switch {
	case strings.Contains(msg, "rawScannerConfig"):
		field = "scanner." + field
	case strings.Contains(msg, "rawKafkaConfig"):
		field = "kafka." + field
	case strings.Contains(msg, "rawChainConfig"):
		field = "chain." + field
	case strings.Contains(msg, "rawRPCConfig"):
		field = "rpc." + field
	case strings.Contains(msg, "rawDatabaseConfig"):
		field = "database." + field
	}
	return fmt.Errorf("invalid YAML configuration: field %s not found", field)
}

func expandBrokers(brokers []string, lookupEnv func(string) (string, bool)) ([]string, error) {
	expanded := make([]string, len(brokers))
	for i, broker := range brokers {
		value, err := expandEnvironment(broker, lookupEnv)
		if err != nil {
			return nil, err
		}
		expanded[i] = value
	}
	return expanded, nil
}

func expandEnvironment(value string, lookupEnv func(string) (string, bool)) (string, error) {
	var missing string
	expanded := environmentVariable.ReplaceAllStringFunc(value, func(match string) string {
		name := environmentVariable.FindStringSubmatch(match)[1]
		replacement, ok := lookupEnv(name)
		if !ok {
			missing = name
			return match
		}
		return replacement
	})
	if missing != "" {
		return "", fmt.Errorf("required environment variable %q is not set", missing)
	}
	if residualEnvironmentVariable.MatchString(expanded) {
		return "", fmt.Errorf("configuration contains an unresolved environment placeholder")
	}
	return expanded, nil
}

func normalizeScanner(raw rawConfig, httpURL, dsn string, brokers []string) (Config, error) {
	chain := normalizeChain(raw.Chain)
	scanner, err := normalizeScannerSection(raw.Scanner)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Chain:    chain,
		RPC:      RPCConfig{HTTPURL: httpURL},
		Scanner:  scanner,
		Database: DatabaseConfig{DSN: dsn},
		Kafka:    normalizeKafka(raw.Kafka, brokers),
	}, nil
}

func normalizeProcessor(raw rawProcessorConfig, dsn string, brokers []string) (ProcessorConfig, error) {
	scanner, err := normalizeScannerSection(raw.Scanner)
	if err != nil {
		return ProcessorConfig{}, err
	}
	return ProcessorConfig{
		Chain:    normalizeChain(raw.Chain),
		Scanner:  scanner,
		Database: DatabaseConfig{DSN: dsn},
		Kafka:    normalizeKafka(raw.Kafka, brokers),
	}, nil
}

func normalizeChain(raw rawChainConfig) ChainConfig {
	startSlot := uint64(0)
	if raw.StartSlot != nil {
		startSlot = *raw.StartSlot
	}
	return ChainConfig{
		Name:        raw.Name,
		GenesisHash: raw.GenesisHash,
		StartSlot:   startSlot,
	}
}

func normalizeScannerSection(raw rawScannerConfig) (ScannerConfig, error) {
	pollInterval, err := time.ParseDuration(raw.PollInterval)
	if err != nil {
		return ScannerConfig{}, fmt.Errorf("invalid configuration: scanner.pollInterval must be a positive duration")
	}
	return ScannerConfig{
		PollInterval:   pollInterval,
		StepSlots:      raw.StepSlots,
		SlotsPerWorker: raw.SlotsPerWorker,
	}, nil
}

func normalizeKafka(raw rawKafkaConfig, brokers []string) KafkaConfig {
	copied := make([]string, len(brokers))
	copy(copied, brokers)
	return KafkaConfig{
		Brokers:          copied,
		BlockTopic:       raw.BlockTopic,
		TransactionTopic: raw.TransactionTopic,
	}
}
