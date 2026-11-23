package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

const (
	base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	minGenesisLen  = 32
	maxGenesisLen  = 44
	minStepSlots   = uint64(1)
	maxStepSlots   = uint64(500000)
)

var (
	base58Runes   = stringSet(base58Alphabet)
	topicPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,249}$`)
	brokerPattern = regexp.MustCompile(`^[^:\s]+:[0-9]+$`)
)

func stringSet(s string) map[rune]struct{} {
	out := make(map[rune]struct{}, len(s))
	for _, r := range s {
		out[r] = struct{}{}
	}
	return out
}

func validateScanner(cfg Config) error {
	if err := validateChain(cfg.Chain); err != nil {
		return err
	}
	if err := validateRPC(cfg.RPC); err != nil {
		return err
	}
	return validateProcess(cfg.Scanner, cfg.Database, cfg.Kafka, false)
}

func validateProcessor(cfg ProcessorConfig) error {
	if err := validateChain(cfg.Chain); err != nil {
		return err
	}
	return validateProcess(cfg.Scanner, cfg.Database, cfg.Kafka, true)
}

func validateProcess(scanner ScannerConfig, database DatabaseConfig, kafka KafkaConfig, requireTransactionTopic bool) error {
	if err := validatePoll(scanner); err != nil {
		return err
	}
	if err := validateSteps(scanner); err != nil {
		return err
	}
	if err := validateDatabase(database); err != nil {
		return err
	}
	return validateKafka(kafka, requireTransactionTopic)
}

func validateSteps(scanner ScannerConfig) error {
	if scanner.StepSlots < minStepSlots || scanner.StepSlots > maxStepSlots {
		return fmt.Errorf("invalid configuration: scanner.stepSlots must be between 1 and 500000")
	}
	if scanner.SlotsPerWorker < minStepSlots || scanner.SlotsPerWorker > maxStepSlots {
		return fmt.Errorf("invalid configuration: scanner.slotsPerWorker must be between 1 and 500000")
	}
	return nil
}

func validateChain(chain ChainConfig) error {
	if chain.Name == "" || containsWhitespace(chain.Name) {
		return fmt.Errorf("invalid configuration: chain.name must be non-empty without whitespace")
	}
	if !isValidGenesisHash(chain.GenesisHash) {
		return fmt.Errorf("invalid configuration: chain.genesisHash must be 32-44 base58 characters")
	}
	return nil
}

func validateRPC(rpc RPCConfig) error {
	parsed, err := url.ParseRequestURI(rpc.HTTPURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid configuration: rpc.httpURL must be an http or https URL with a host")
	}
	return nil
}

func validatePoll(scanner ScannerConfig) error {
	if scanner.PollInterval <= 0 {
		return fmt.Errorf("invalid configuration: scanner.pollInterval must be greater than 0")
	}
	return nil
}

func validateDatabase(database DatabaseConfig) error {
	if strings.TrimSpace(database.DSN) == "" {
		return fmt.Errorf("invalid configuration: database.dsn is required")
	}
	return nil
}

func validateKafka(kafka KafkaConfig, requireTransactionTopic bool) error {
	if len(kafka.Brokers) == 0 {
		return fmt.Errorf("invalid configuration: kafka.brokers must be non-empty")
	}
	for _, broker := range kafka.Brokers {
		if !brokerPattern.MatchString(broker) {
			return fmt.Errorf("invalid configuration: kafka.brokers entries must be host:port")
		}
	}
	if requireTransactionTopic || kafka.TransactionTopic != "" {
		if !topicPattern.MatchString(kafka.TransactionTopic) {
			return fmt.Errorf("invalid configuration: kafka.transactionTopic must match [A-Za-z0-9._-]{1,249}")
		}
	}
	if !topicPattern.MatchString(kafka.BlockTopic) {
		return fmt.Errorf("invalid configuration: kafka.blockTopic must match [A-Za-z0-9._-]{1,249}")
	}
	return nil
}

func containsWhitespace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func isValidGenesisHash(value string) bool {
	if len(value) < minGenesisLen || len(value) > maxGenesisLen {
		return false
	}
	for _, r := range value {
		if _, ok := base58Runes[r]; !ok {
			return false
		}
	}
	return true
}
