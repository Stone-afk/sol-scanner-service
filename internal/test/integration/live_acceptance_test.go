//go:build e2e

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/transaction"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/postgre"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	repopostgre "github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
	"github.com/Stone-afk/sol-scanner-service/internal/test/jsonparsed"
	"github.com/Stone-afk/sol-scanner-service/internal/test/lagcheck"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"gorm.io/gorm"
)

// Live read-only acceptance (design §6.6 A-01..A-04 / AC-073..AC-076, AC-097).
// Skips unless SOL_SCANNER_LIVE_E2E=1. Non-local Postgres DSN fails before dial.
// Never log full URL, DSN, or API keys.

const (
	liveEnvVar           = "SOL_SCANNER_LIVE_E2E"
	liveConfigEnvVar     = "SOL_SCANNER_LIVE_CONFIG"
	liveSlotCount        = uint64(200)
	liveSampleBlocks     = 20
	liveLagObserveWindow = 10 * time.Minute
	liveProcessWait      = 45 * time.Minute
	livePollInterval     = 2 * time.Second
	liveProbeSlots       = uint64(40)
	// Live run overrides: keep steps small so first produce beats empty-poll gap + 429.
	liveStepSlots      = "20"
	liveSlotsPerWorker = "10"
	liveScannerPoll    = "5s" // after queue has traffic; 10 empty ≈ 50s safety margin
	liveQueueWarmWait  = 20 * time.Minute
	// shortcut: mainnet getBlock JSON often 1.5–4MiB; 32MiB is e2e/prod client cap — raise + retest if larger blocks appear.
	liveKafkaMaxBytes = 32 * 1024 * 1024
)

func TestLiveAcceptance(t *testing.T) {
	// AC-097 gate: always runs in-process without setting the live env or dialing.
	t.Run("AC097_reject_non_local_dsn", testRejectNonLocalPostgresDSN)

	if os.Getenv(liveEnvVar) != "1" {
		t.Skip("SOL_SCANNER_LIVE_E2E is not 1; skipping mainnet read-only acceptance (AC-097)")
	}
	runLiveAcceptance(t)
}

func testRejectNonLocalPostgresDSN(t *testing.T) {
	cases := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{"remote_url", "postgres://u:p@db.example.test:5432/sol_scanner?sslmode=disable", true},
		{"remote_host_kv", "host=db.example.test user=u password=secret dbname=sol_scanner", true},
		{"remote_ip", "postgres://u:p@203.0.113.10:5432/sol?sslmode=disable", true},
		{"loopback_url", "postgres://u:p@127.0.0.1:5432/sol?sslmode=disable", false},
		{"localhost_url", "postgres://u:p@localhost:5432/sol?sslmode=disable", false},
		{"localhost_kv", "host=localhost user=u password=secret dbname=sol", false},
		{"loopback_kv", "host=127.0.0.1 port=5432 user=u dbname=sol", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dialed := false
			err := withLocalPostgresDSN(tc.dsn, func(string) error {
				dialed = true
				return nil
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected non-local DSN rejection")
				}
				if dialed {
					t.Fatal("dial callback ran before DSN rejection")
				}
				msg := err.Error()
				for _, leak := range []string{"postgres://", "password=", "db.example.test", "203.0.113.10", "secret"} {
					if strings.Contains(strings.ToLower(msg), strings.ToLower(leak)) {
						t.Fatalf("rejection error must not contain DSN secrets")
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("local DSN rejected: %v", err)
			}
			if !dialed {
				t.Fatal("expected dial callback after local DSN gate")
			}
		})
	}
}

// withLocalPostgresDSN enforces AC-097 before dial. dial must not run for remote hosts.
func withLocalPostgresDSN(dsn string, dial func(dsn string) error) error {
	if err := requireLocalPostgresDSN(dsn); err != nil {
		return err
	}
	return dial(dsn)
}

func requireLocalPostgresDSN(dsn string) error {
	host, err := postgresDSNHost(dsn)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && !strings.EqualFold(host, "localhost") {
		return errors.New("live acceptance refused: database host must be 127.0.0.1 or localhost")
	}
	return nil
}

func postgresDSNHost(dsn string) (string, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "", errors.New("live acceptance refused: empty database DSN")
	}
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", errors.New("live acceptance refused: unparseable database DSN")
		}
		if host := u.Hostname(); host != "" {
			return host, nil
		}
		return "localhost", nil // URL without host → Unix socket / local default
	}
	for _, field := range strings.Fields(dsn) {
		key, value, ok := strings.Cut(field, "=")
		if ok && strings.EqualFold(key, "host") {
			if value == "" {
				return "", errors.New("live acceptance refused: empty database host")
			}
			return value, nil
		}
	}
	return "localhost", nil // libpq default
}

func liveConfigPath(t *testing.T) string {
	t.Helper()
	if p := strings.TrimSpace(os.Getenv(liveConfigEnvVar)); p != "" {
		return p
	}
	return filepath.Join(repositoryRoot(t), "config", "config.local.yaml")
}

func runLiveAcceptance(t *testing.T) {
	t.Helper()
	srcPath := liveConfigPath(t)
	baseCfg, err := config.LoadForScanner(srcPath, os.LookupEnv)
	if err != nil {
		t.Fatalf("load live configuration: %v", err)
	}

	var db *gorm.DB
	err = withLocalPostgresDSN(baseCfg.Database.DSN, func(dsn string) error {
		opened, openErr := postgre.Open(dsn)
		if openErr != nil {
			return openErr
		}
		db = opened
		return nil
	})
	if err != nil {
		t.Fatalf("live database gate/open: %v", err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = postgre.Close(db)
		}
	})
	if err := postgre.Ping(context.Background(), db); err != nil {
		t.Fatalf("ping local database: %v", err)
	}
	if err := ensureMigrations(t, db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	clearUnresolvedHalts(t, db)

	reader, err := provider.NewReader(baseCfg.RPC.HTTPURL)
	if err != nil {
		t.Fatalf("construct RPC reader: %v", err)
	}
	// A-04 needs tip proximity; historical startSlot stays hundreds of thousands behind.
	finalized, err := reader.FinalizedSlot(context.Background())
	if err != nil {
		t.Fatalf("FinalizedSlot for live start: %v", err)
	}
	pad := liveSlotCount + 50
	if finalized <= pad {
		t.Fatalf("finalized slot %d too low for live window", finalized)
	}
	liveStart := finalized - pad
	cfgPath := writeLiveTunedConfig(t, srcPath, liveStart)
	cfg, err := config.LoadForScanner(cfgPath, os.LookupEnv)
	if err != nil {
		t.Fatalf("load tuned live configuration: %v", err)
	}
	t.Logf("live tuned scanner: startSlot=%d stepSlots=%d slotsPerWorker=%d pollInterval=%s (finalized=%d)",
		cfg.Chain.StartSlot, cfg.Scanner.StepSlots, cfg.Scanner.SlotsPerWorker, cfg.Scanner.PollInterval, finalized)

	ensureKafkaTopics(t, cfg.Kafka)

	h := &liveHarness{
		t:          t,
		root:       repositoryRoot(t),
		configPath: cfgPath,
		cfg:        cfg,
		db:         db,
		reader:     reader,
		scanRepo:   repository.NewScanRepository(repopostgre.NewScanDAO(db)),
	}
	h.startBinaries()
	t.Cleanup(h.stopBinaries)

	slotRange := h.waitForSlotWindow(liveSlotCount)
	t.Run("A01_blocks_and_parent_continuity", func(t *testing.T) { h.assertA01(t, slotRange) })
	t.Run("A02_jsonParsed_sample", func(t *testing.T) { h.assertA02(t, slotRange) })
	t.Run("A03_kafka_tx_dedup", func(t *testing.T) { h.assertA03(t, slotRange) })
	t.Run("A04_lag_observation", func(t *testing.T) { h.assertA04(t) })
}

// writeLiveTunedConfig copies the operator local file with small steps and a tip-near startSlot.
func writeLiveTunedConfig(t *testing.T, src string, startSlot uint64) string {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read live config: %v", err)
	}
	text := string(raw)
	text = replaceYAMLKey(text, "stepSlots", liveStepSlots)
	text = replaceYAMLKey(text, "slotsPerWorker", liveSlotsPerWorker)
	text = replaceYAMLKey(text, "pollInterval", liveScannerPoll)
	text = replaceYAMLKey(text, "startSlot", fmt.Sprintf("%d", startSlot))
	dst := filepath.Join(t.TempDir(), "config.live.yaml")
	if err := os.WriteFile(dst, []byte(text), 0o600); err != nil {
		t.Fatalf("write live tuned config: %v", err)
	}
	return dst
}

func replaceYAMLKey(text, key, value string) string {
	re := regexp.MustCompile(`(?m)^(\s*` + regexp.QuoteMeta(key) + `:\s*)\S+`)
	if !re.MatchString(text) {
		return text
	}
	return re.ReplaceAllString(text, `${1}`+value)
}

func clearUnresolvedHalts(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now().Unix()
	res := db.Exec(`UPDATE sol_scan_halts SET resolved_at = ? WHERE resolved_at IS NULL`, now)
	if res.Error != nil {
		t.Fatalf("clear unresolved halts: %v", res.Error)
	}
	if res.RowsAffected > 0 {
		t.Logf("live harness: resolved %d leftover halt row(s)", res.RowsAffected)
	}
}

func ensureKafkaTopics(t *testing.T, cfg config.KafkaConfig) {
	t.Helper()
	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": strings.Join(cfg.Brokers, ","),
	})
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer admin.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	topicCfg := map[string]string{"max.message.bytes": fmt.Sprintf("%d", liveKafkaMaxBytes)}
	results, err := admin.CreateTopics(ctx, []kafka.TopicSpecification{
		{Topic: cfg.BlockTopic, NumPartitions: 1, ReplicationFactor: 1, Config: topicCfg},
		{Topic: cfg.TransactionTopic, NumPartitions: 1, ReplicationFactor: 1, Config: topicCfg},
	})
	if err != nil {
		t.Fatalf("CreateTopics: %v", err)
	}
	for _, r := range results {
		if r.Error.Code() != kafka.ErrNoError && r.Error.Code() != kafka.ErrTopicAlreadyExists {
			t.Fatalf("CreateTopics %s: %v", r.Topic, r.Error)
		}
	}
	t.Logf("live harness: kafka topics ready (%s, %s)", cfg.BlockTopic, cfg.TransactionTopic)
}

func ensureMigrations(t *testing.T, db *gorm.DB) error {
	t.Helper()
	runner, err := postgre.NewMigrationRunner(db, migrationDir(t))
	if err != nil {
		return err
	}
	ctx := context.Background()
	for {
		executed, err := runner.Up(ctx)
		if err != nil {
			return err
		}
		if executed == nil {
			break
		}
	}
	return postgre.CheckSchemaVersion(ctx, db, 2)
}

type liveSlotRange struct{ From, To uint64 }

type liveHarness struct {
	t          *testing.T
	root       string
	configPath string
	cfg        config.Config
	db         *gorm.DB
	reader     provider.Reader
	scanRepo   repository.ScanRepository
	scannerCmd *exec.Cmd
	procCmd    *exec.Cmd
}

func (h *liveHarness) startBinaries() {
	h.t.Helper()
	binDir := h.t.TempDir()
	scannerBin := filepath.Join(binDir, "sol-scanner")
	processorBin := filepath.Join(binDir, "sol-processor")
	h.mustBuild(scannerBin, "./cmd/sol-scanner")
	h.mustBuild(processorBin, "./cmd/sol-processor")
	h.scannerCmd = h.command(scannerBin)
	h.procCmd = h.command(processorBin)
	// Scanner first: processor empty-poll GAP must not fire before the queue has traffic.
	if err := h.scannerCmd.Start(); err != nil {
		h.t.Fatalf("start sol-scanner: %v", err)
	}
	h.waitForBlockQueueTraffic(liveQueueWarmWait)
	if err := h.procCmd.Start(); err != nil {
		_ = h.scannerCmd.Process.Kill()
		h.t.Fatalf("start sol-processor: %v", err)
	}
}

func (h *liveHarness) waitForBlockQueueTraffic(timeout time.Duration) {
	h.t.Helper()
	kc, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(h.cfg.Kafka.Brokers, ","),
		"group.id":           fmt.Sprintf("live-warm-%d", time.Now().UnixNano()),
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
	})
	if err != nil {
		h.t.Fatalf("warm consumer: %v", err)
	}
	defer kc.Close()
	if err := kc.SubscribeTopics([]string{h.cfg.Kafka.BlockTopic}, nil); err != nil {
		h.t.Fatalf("warm subscribe: %v", err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.scannerCmd.ProcessState != nil && h.scannerCmd.ProcessState.Exited() {
			h.t.Fatal("sol-scanner exited before producing block-queue traffic")
		}
		ev := kc.Poll(500)
		switch e := ev.(type) {
		case *kafka.Message:
			h.t.Logf("live harness: block queue has traffic (slot key=%s); starting processor", string(e.Key))
			return
		case kafka.Error:
			if e.IsTimeout() || e.Code() == kafka.ErrUnknownTopic || e.Code() == kafka.ErrUnknownTopicOrPart {
				continue
			}
			h.t.Logf("live harness: warm poll kafka error: %v", e)
		}
	}
	h.t.Fatalf("timeout waiting for first block-queue message (scanner still rate-limited or stuck?)")
}

func (h *liveHarness) mustBuild(out, packagePath string) {
	h.t.Helper()
	cmd := exec.Command("go", "build", "-o", out, packagePath)
	cmd.Dir = h.root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		h.t.Fatalf("go build %s: %v", packagePath, err)
	}
}

func (h *liveHarness) command(bin string) *exec.Cmd {
	cmd := exec.Command(bin, "--config", h.configPath)
	cmd.Dir = h.root
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (h *liveHarness) stopBinaries() {
	cmds := []*exec.Cmd{h.scannerCmd, h.procCmd}
	h.scannerCmd, h.procCmd = nil, nil // idempotent: Cleanup may call again after assertA04
	for _, cmd := range cmds {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
	}
	done := make(chan struct{}, 2)
	waiting := 0
	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		waiting++
		go func(c *exec.Cmd) { _ = c.Wait(); done <- struct{}{} }(cmd)
	}
	timeout := time.After(15 * time.Second)
	for i := 0; i < waiting; i++ {
		select {
		case <-done:
		case <-timeout:
			for _, cmd := range cmds {
				if cmd != nil && cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			}
			return
		}
	}
}

func (h *liveHarness) waitForSlotWindow(count uint64) liveSlotRange {
	h.t.Helper()
	ctx := context.Background()
	from := h.cfg.Chain.StartSlot
	if cp, ok, err := h.scanRepo.GetCheckpoint(ctx); err != nil {
		h.t.Fatalf("GetCheckpoint: %v", err)
	} else if ok {
		from = cp.Slot + 1
	}
	if from == 0 {
		h.t.Fatal("live acceptance needs chain.startSlot or an existing checkpoint")
	}
	to := from + count - 1
	deadline := time.Now().Add(liveProcessWait)
	for time.Now().Before(deadline) {
		cp, ok, err := h.scanRepo.GetCheckpoint(ctx)
		if err != nil {
			h.t.Fatalf("GetCheckpoint while waiting: %v", err)
		}
		if ok && cp.Slot >= to {
			h.t.Logf("live harness: processed slot window [%d,%d]", from, to)
			return liveSlotRange{From: from, To: to}
		}
		time.Sleep(livePollInterval)
	}
	h.t.Fatalf("timeout waiting for %d slots starting at %d", count, from)
	return liveSlotRange{}
}

type storedBlockRow struct {
	Slot              uint64
	Blockhash         string
	ParentSlot        uint64
	PreviousBlockhash string
	TransactionCount  int
}

func (h *liveHarness) loadStoredBlocks(t *testing.T, from, to uint64) []storedBlockRow {
	t.Helper()
	var rows []storedBlockRow
	if err := h.db.WithContext(context.Background()).
		Table("sol_blocks").
		Select("slot, blockhash, parent_slot, previous_blockhash, transaction_count").
		Where("slot >= ? AND slot <= ?", from, to).
		Order("slot ASC").
		Find(&rows).Error; err != nil {
		t.Fatalf("query sol_blocks: %v", err)
	}
	return rows
}

func (h *liveHarness) assertA01(t *testing.T, r liveSlotRange) {
	t.Helper()
	wantSlots, err := h.reader.BlockSlots(context.Background(), r.From, r.To)
	if err != nil {
		t.Fatalf("A-01 getBlocks: %v", err)
	}
	stored := h.loadStoredBlocks(t, r.From, r.To)
	if len(stored) != len(wantSlots) {
		t.Fatalf("A-01 stored block count %d != getBlocks %d", len(stored), len(wantSlots))
	}
	for i := range wantSlots {
		if stored[i].Slot != wantSlots[i] {
			t.Fatalf("A-01 slot mismatch at %d: stored=%d getBlocks=%d", i, stored[i].Slot, wantSlots[i])
		}
	}
	for i := 1; i < len(stored); i++ {
		prev, cur := stored[i-1], stored[i]
		if cur.ParentSlot != prev.Slot {
			t.Fatalf("A-01 parent slot gap: slot %d parent_slot=%d want %d", cur.Slot, cur.ParentSlot, prev.Slot)
		}
		if cur.PreviousBlockhash != prev.Blockhash {
			t.Fatalf("A-01 parent hash mismatch at slot %d", cur.Slot)
		}
	}
	t.Logf("A-01 passed for slots [%d,%d] (%d blocks)", r.From, r.To, len(stored))
}

func (h *liveHarness) assertA02(t *testing.T, r liveSlotRange) {
	t.Helper()
	stored := h.loadStoredBlocks(t, r.From, r.To)
	if len(stored) == 0 {
		t.Fatal("A-02: no stored blocks to sample")
	}
	step := len(stored) / liveSampleBlocks
	if step < 1 {
		step = 1
	}
	sampled := 0
	for i := 0; i < len(stored) && sampled < liveSampleBlocks; i += step {
		row := stored[i]
		oracle := h.fetchJSONParsedBlock(t, row.Slot)
		if len(oracle.Transactions) != row.TransactionCount {
			t.Fatalf("A-02 slot %d tx count: db_blocks=%d jsonParsed=%d",
				row.Slot, row.TransactionCount, len(oracle.Transactions))
		}
		var dbTxs []struct {
			TxIndex   int
			Signature string
			Status    int16
			Fee       uint64
		}
		if err := h.db.WithContext(context.Background()).
			Table("sol_transactions").
			Select("tx_index, signature, status, fee").
			Where("slot = ?", row.Slot).
			Order("tx_index ASC").
			Find(&dbTxs).Error; err != nil {
			t.Fatalf("A-02 query transactions slot=%d: %v", row.Slot, err)
		}
		if len(dbTxs) != len(oracle.Transactions) {
			t.Fatalf("A-02 slot %d transaction rows %d != jsonParsed %d",
				row.Slot, len(dbTxs), len(oracle.Transactions))
		}
		for j, want := range oracle.Transactions {
			got := dbTxs[j]
			if got.TxIndex != j {
				t.Fatalf("A-02 slot %d tx_index gap at %d: db=%d", row.Slot, j, got.TxIndex)
			}
			if got.Signature != want.Signature || got.Fee != want.Fee {
				t.Fatalf("A-02 slot %d tx %d signature/fee mismatch", row.Slot, j)
			}
			wantStatus := int16(domain.TxStatusSuccess)
			if want.Failed {
				wantStatus = int16(domain.TxStatusFailed)
			}
			if got.Status != wantStatus {
				t.Fatalf("A-02 slot %d tx %d status: db=%d want=%d", row.Slot, j, got.Status, wantStatus)
			}
			h.assertA02Transfers(t, row.Slot, j, want)
			h.assertA02TokenBalances(t, row.Slot, j, want)
		}
		sampled++
	}
	if sampled == 0 {
		t.Fatal("A-02 sampled zero blocks")
	}
	t.Logf("A-02 sampled %d blocks vs independent jsonParsed in [%d,%d]", sampled, r.From, r.To)
}

// fetchJSONParsedBlock calls Infura getBlock with encoding=jsonParsed (not provider.Block/base64).
func (h *liveHarness) fetchJSONParsedBlock(t *testing.T, slot uint64) jsonparsed.ParsedBlock {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBlock",
		"params": []any{
			slot,
			map[string]any{
				"encoding":                       "jsonParsed",
				"commitment":                     "finalized",
				"maxSupportedTransactionVersion": 1,
				"transactionDetails":             "full",
				"rewards":                        false,
			},
		},
	})
	if err != nil {
		t.Fatalf("A-02 marshal getBlock: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.RPC.HTTPURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("A-02 NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("A-02 jsonParsed getBlock slot=%d: request failed", slot)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		t.Fatalf("A-02 read jsonParsed slot=%d: %v", slot, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("A-02 jsonParsed getBlock slot=%d: HTTP %d", slot, resp.StatusCode)
	}
	parsed, err := jsonparsed.ParseParsedBlockJSON(body)
	if err != nil {
		t.Fatalf("A-02 ParseParsedBlockJSON slot=%d: %v", slot, err)
	}
	return parsed
}

func (h *liveHarness) assertA02Transfers(t *testing.T, slot uint64, txIndex int, want jsonparsed.ParsedTx) {
	t.Helper()
	oracle := jsonparsed.Transfers(want)
	var dbRows []struct {
		IxIndex     int16
		InnerIndex  int16
		Program     int16
		Kind        int16
		FromAccount string
		ToAccount   string
		Amount      string
	}
	if err := h.db.WithContext(context.Background()).
		Table("sol_transfers").
		Select("ix_index, inner_index, program, kind, from_account, to_account, amount::text AS amount").
		Where("slot = ? AND tx_index = ?", slot, txIndex).
		Order("ix_index ASC, inner_index ASC").
		Find(&dbRows).Error; err != nil {
		t.Fatalf("A-02 query transfers slot=%d tx=%d: %v", slot, txIndex, err)
	}
	if want.Failed {
		if len(dbRows) != 0 {
			t.Fatalf("A-02 slot %d tx %d failed but has %d transfers", slot, txIndex, len(dbRows))
		}
		return
	}
	if len(dbRows) != len(oracle) {
		t.Fatalf("A-02 slot %d tx %d transfer count: db=%d jsonParsed=%d",
			slot, txIndex, len(dbRows), len(oracle))
	}
	for i, o := range oracle {
		got := dbRows[i]
		idx := liveTransferIndex(int(got.IxIndex), int(got.InnerIndex))
		if idx != o.Index ||
			liveProgramName(got.Program) != o.Program ||
			liveKindName(got.Kind) != o.Kind ||
			got.FromAccount != o.From ||
			got.ToAccount != o.To ||
			got.Amount != o.Amount {
			t.Fatalf("A-02 slot %d tx %d transfer[%d] mismatch", slot, txIndex, i)
		}
	}
}

func (h *liveHarness) assertA02TokenBalances(t *testing.T, slot uint64, txIndex int, want jsonparsed.ParsedTx) {
	t.Helper()
	oracle := jsonparsed.Token2022BalanceChanges(want)
	var dbRows []struct {
		AccountIndex int
		Account      string
		Owner        string
		Mint         string
		Decimals     int16
		PreAmount    string
		PostAmount   string
	}
	if err := h.db.WithContext(context.Background()).
		Table("sol_token_balance_changes").
		Select("account_index, account, owner, mint, decimals, pre_amount::text AS pre_amount, post_amount::text AS post_amount").
		Where("slot = ? AND tx_index = ?", slot, txIndex).
		Order("account_index ASC").
		Find(&dbRows).Error; err != nil {
		t.Fatalf("A-02 query token balances slot=%d tx=%d: %v", slot, txIndex, err)
	}
	if len(dbRows) != len(oracle) {
		t.Fatalf("A-02 slot %d tx %d token-balance count: db=%d jsonParsed=%d",
			slot, txIndex, len(dbRows), len(oracle))
	}
	for i, o := range oracle {
		got := dbRows[i]
		if got.AccountIndex != o.AccountIndex ||
			got.Account != o.Account ||
			got.Owner != o.Owner ||
			got.Mint != o.Mint ||
			uint8(got.Decimals) != o.Decimals ||
			got.PreAmount != o.PreAmount ||
			got.PostAmount != o.PostAmount {
			t.Fatalf("A-02 slot %d tx %d token-balance[%d] mismatch", slot, txIndex, i)
		}
	}
}

func liveTransferIndex(ixIndex, innerIndex int) string {
	if innerIndex < 0 {
		return strconv.Itoa(ixIndex + 1)
	}
	return fmt.Sprintf("%d.%d", ixIndex+1, innerIndex+1)
}

func liveProgramName(p int16) string {
	switch domain.Program(p) {
	case domain.ProgramSystem:
		return "system"
	case domain.ProgramSPLToken:
		return "spl-token"
	case domain.ProgramToken2022:
		return "token-2022"
	default:
		return fmt.Sprintf("program(%d)", p)
	}
}

func liveKindName(k int16) string {
	switch domain.TransferKind(k) {
	case domain.KindTransfer:
		return "transfer"
	case domain.KindTransferWithSeed:
		return "transferWithSeed"
	case domain.KindCreateAccount:
		return "createAccount"
	case domain.KindCreateAccountWithSeed:
		return "createAccountWithSeed"
	case domain.KindWithdrawNonceAccount:
		return "withdrawNonceAccount"
	case domain.KindTransferChecked:
		return "transferChecked"
	case domain.KindTransferCheckedWithFee:
		return "transferCheckedWithFee"
	case domain.KindMintTo:
		return "mintTo"
	case domain.KindMintToChecked:
		return "mintToChecked"
	case domain.KindBurn:
		return "burn"
	case domain.KindBurnChecked:
		return "burnChecked"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

func (h *liveHarness) assertA03(t *testing.T, r liveSlotRange) {
	t.Helper()
	var dbTxCount int64
	if err := h.db.WithContext(context.Background()).
		Table("sol_transactions").
		Where("slot >= ? AND slot <= ?", r.From, r.To).
		Count(&dbTxCount).Error; err != nil {
		t.Fatalf("A-03 count transactions: %v", err)
	}
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(h.cfg.Kafka.Brokers, ","),
		"group.id":           fmt.Sprintf("live-a03-%d", time.Now().UnixNano()),
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
	})
	if err != nil {
		t.Fatalf("A-03 NewConsumer: %v", err)
	}
	defer consumer.Close()
	if err := consumer.Subscribe(h.cfg.Kafka.TransactionTopic, nil); err != nil {
		t.Fatalf("A-03 Subscribe: %v", err)
	}
	seen := make(map[string]struct{})
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) && int64(len(seen)) < dbTxCount {
		ev := consumer.Poll(500)
		msg, ok := ev.(*kafka.Message)
		if !ok || msg == nil {
			continue
		}
		var tm transaction.TransactionMessage
		if err := json.Unmarshal(msg.Value, &tm); err != nil {
			t.Fatalf("A-03 unmarshal transaction message: %v", err)
		}
		if tm.Slot < r.From || tm.Slot > r.To {
			continue
		}
		if tm.Signature == "" || tm.Version != transaction.MessageVersion {
			t.Fatalf("A-03 invalid message shape")
		}
		seen[tm.Signature] = struct{}{}
	}
	if int64(len(seen)) != dbTxCount {
		t.Fatalf("A-03 deduped kafka messages %d != db transactions %d", len(seen), dbTxCount)
	}
	t.Logf("A-03 deduped message count matches %d transactions", dbTxCount)
}

func (h *liveHarness) assertA04(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	src := lagcheck.Source{
		FinalizedSlot: h.reader.FinalizedSlot,
		Checkpoint: func(ctx context.Context) (uint64, bool, error) {
			cp, ok, err := h.scanRepo.GetCheckpoint(ctx)
			if err != nil || !ok {
				return 0, ok, err
			}
			return cp.Slot, true, nil
		},
	}
	clock := lagcheck.Clock{Now: time.Now, Sleep: liveSleepCtx}
	obs := lagcheck.Observe(ctx, src, liveLagObserveWindow, livePollInterval, clock)
	h.stopBinaries()
	probe := h.throughputProbe(ctx, t)
	gate := lagcheck.ClassifyThroughput(probe, obs)
	verdict := lagcheck.Judge(obs, gate)
	fa04 := lagcheck.FAStatus(verdict, gate)
	tipRate, probeRate := a04Rates(obs, probe)
	line := fmt.Sprintf(
		"A-04 verdict=%s gate=%s fa04=%s samples=%d last_lag=%d max_lag=%d limit_signals=%d tip_rate=%.2f probe_rate=%.2f",
		verdict, gate, fa04, obs.Samples, obs.LastLag, obs.MaxLag, obs.LimitSignals, tipRate, probeRate,
	)
	if verdict == lagcheck.VerdictFail {
		t.Fatalf("%s", line)
	}
	t.Logf("%s", line)
}

func liveSleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func a04Rates(obs lagcheck.Observation, p lagcheck.Probe) (tipRate, probeRate float64) {
	if obs.Elapsed > 0 && obs.TipSamples >= 2 {
		tipRate = float64(obs.LastTip-obs.FirstTip) / obs.Elapsed.Seconds()
	}
	if p.Elapsed > 0 {
		probeRate = float64(p.Slots) / p.Elapsed.Seconds()
	}
	return tipRate, probeRate
}

func (h *liveHarness) throughputProbe(ctx context.Context, t *testing.T) lagcheck.Probe {
	t.Helper()
	finalized, err := h.reader.FinalizedSlot(ctx)
	if err != nil {
		if lagcheck.IsRateLimit(err) {
			return lagcheck.Probe{Slots: liveProbeSlots, LimitSignals: 1}
		}
		t.Fatalf("A-04 probe FinalizedSlot failed")
	}
	from := uint64(0)
	if finalized+1 >= liveProbeSlots {
		from = finalized - (liveProbeSlots - 1)
	}
	step := h.cfg.Scanner.StepSlots
	per := h.cfg.Scanner.SlotsPerWorker
	nWorkers := int((step + per - 1) / per)
	if nWorkers < 1 {
		nWorkers = 1
	}
	ranges := splitInclusive(from, finalized, nWorkers)

	start := time.Now()
	var (
		mu           sync.Mutex
		limitSignals int
		wg           sync.WaitGroup
		fatalOnce    sync.Once
		fatalErr     error
	)
	for _, rg := range ranges {
		wg.Add(1)
		go func(left, right uint64) {
			defer wg.Done()
			if err := h.probeSegment(ctx, left, right, &mu, &limitSignals); err != nil {
				fatalOnce.Do(func() { fatalErr = err })
			}
		}(rg[0], rg[1])
	}
	wg.Wait()
	if fatalErr != nil {
		t.Fatalf("A-04 probe RPC failed")
	}
	return lagcheck.Probe{
		Slots:        liveProbeSlots,
		Elapsed:      time.Since(start),
		LimitSignals: limitSignals,
	}
}

func (h *liveHarness) probeSegment(ctx context.Context, left, right uint64, mu *sync.Mutex, limits *int) error {
	slots, err := h.reader.BlockSlots(ctx, left, right)
	if err != nil {
		if lagcheck.IsRateLimit(err) {
			mu.Lock()
			*limits++
			mu.Unlock()
			return nil
		}
		return err
	}
	present := make(map[uint64]struct{}, len(slots))
	for _, slot := range slots {
		present[slot] = struct{}{}
	}
	for slot := left; slot <= right; slot++ {
		if _, ok := present[slot]; !ok {
			continue
		}
		if _, err := h.reader.Block(ctx, slot); err != nil {
			if lagcheck.IsRateLimit(err) {
				mu.Lock()
				*limits++
				mu.Unlock()
				return nil
			}
			return err
		}
	}
	return nil
}

func splitInclusive(from, to uint64, n int) [][2]uint64 {
	if from > to {
		return nil
	}
	total := to - from + 1
	if n < 1 {
		n = 1
	}
	if uint64(n) > total {
		n = int(total)
	}
	out := make([][2]uint64, 0, n)
	base := total / uint64(n)
	extra := total % uint64(n)
	cur := from
	for i := 0; i < n; i++ {
		size := base
		if uint64(i) < extra {
			size++
		}
		end := cur + size - 1
		out = append(out, [2]uint64{cur, end})
		cur = end + 1
	}
	return out
}
