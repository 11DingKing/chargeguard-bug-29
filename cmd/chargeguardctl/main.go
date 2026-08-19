package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"chargeguard/internal/config"
	"chargeguard/internal/domain"
	"chargeguard/internal/service"
	"chargeguard/internal/storage"
)

var dataDir string
var configPath string

func main() {
	rootCmd := &cobra.Command{
		Use:   "chargeguardctl",
		Short: "Operations CLI for postal hub",
	}
	rootCmd.PersistentFlags().StringVarP(&dataDir, "data-dir", "d", "", "data directory (overrides config)")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "configs/config.yaml", "config file path")
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(importCmd())
	rootCmd.AddCommand(exportCmd())
	rootCmd.AddCommand(verifyCmd())
	rootCmd.AddCommand(rebuildCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(diagnoseCmd())
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func loadConfig() (config.Config, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return cfg, err
	}
	if dataDir != "" {
		cfg.DataDir = dataDir
	}
	return cfg, nil
}

func connect() (storage.Store, config.Config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, cfg, err
	}
	store, err := storage.NewStore(context.Background(), cfg.DataDir, domain.RealClock{})
	if err != nil {
		return nil, cfg, fmt.Errorf("open store: %w", err)
	}
	return store, cfg, nil
}

func withStore(timeout time.Duration, fn func(ctx context.Context, store storage.Store, cfg config.Config, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		store, cfg, err := connect()
		if err != nil {
			return err
		}
		defer store.Close()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return fn(ctx, store, cfg, args)
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize data directory and database schema",
		RunE: withStore(30*time.Second, func(ctx context.Context, store storage.Store, cfg config.Config, args []string) error {
			fmt.Printf("Initialized data directory: %s\n", cfg.DataDir)
			fmt.Printf("Manifest database: %s/manifest.db\n", cfg.DataDir)
			fmt.Printf("Shards directory: %s/shards/\n", cfg.DataDir)
			return nil
		}),
	}
}

func importCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import handover lists from CSV or JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: withStore(5*time.Minute, func(ctx context.Context, store storage.Store, _ config.Config, args []string) error {
			eventBus := service.NewEventBus(store.EventRepo())
			handSvc := service.NewHandoverService(store, domain.RealClock{}, eventBus)
			importSvc := service.NewImportExportService(handSvc, domain.RealClock{})
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			var result *service.ImportResult
			if format == "json" || strings.HasSuffix(args[0], ".json") {
				result, err = importSvc.ImportHandoversJSON(ctx, f)
			} else {
				result, err = importSvc.ImportHandoversCSV(ctx, f)
			}
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(result)
			fmt.Printf("\nImported: %d succeeded, %d failed out of %d total\n", result.Succeeded, result.Failed, result.TotalRows)
			return nil
		}),
	}
	cmd.Flags().StringVarP(&format, "format", "f", "", "input format: csv or json")
	return cmd
}

func exportCmd() *cobra.Command {
	var date, routeID, outFile string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export ledger entries to CSV file",
		RunE: withStore(30*time.Second, func(ctx context.Context, store storage.Store, _ config.Config, _ []string) error {
			ledgerSvc := service.NewLedgerService(store, domain.RealClock{})
			csvData, err := ledgerSvc.ExportCSV(ctx, date, routeID)
			if err != nil {
				return err
			}
			if outFile != "" {
				return os.WriteFile(outFile, []byte(csvData), 0o644)
			}
			fmt.Print(csvData)
			return nil
		}),
	}
	cmd.Flags().StringVar(&date, "date", "", "ledger date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&routeID, "route", "", "route ID")
	cmd.Flags().StringVarP(&outFile, "output", "o", "", "output file path")
	return cmd
}

func verifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify data integrity: shard checksums and consistency",
		RunE: withStore(5*time.Minute, func(ctx context.Context, store storage.Store, _ config.Config, _ []string) error {
			maintSvc := service.NewMaintenanceService(store, domain.RealClock{})
			report, err := maintSvc.Verify(ctx)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(report)
			fmt.Printf("\nShards: %d total, %d OK, %d damaged\n", report.TotalShards, report.OKShards, report.DamagedShards)
			if len(report.Errors) > 0 {
				fmt.Printf("Errors: %d\n", len(report.Errors))
				for _, e := range report.Errors {
					fmt.Printf("  - %s\n", e)
				}
			}
			return nil
		}),
	}
}

func rebuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild manifest index from shard files",
		RunE: withStore(30*time.Minute, func(ctx context.Context, store storage.Store, _ config.Config, _ []string) error {
			maintSvc := service.NewMaintenanceService(store, domain.RealClock{})
			report, err := maintSvc.RebuildIndex(ctx)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(report)
			fmt.Printf("\nRebuilt: %d/%d shards, %d records\n", report.RebuiltShards, report.TotalShards, report.TotalRecords)
			if len(report.DamagedShards) > 0 {
				fmt.Printf("Damaged shards: %v\n", report.DamagedShards)
			}
			return nil
		}),
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show service status: database, shards, and record counts",
		RunE: withStore(30*time.Second, func(ctx context.Context, store storage.Store, cfg config.Config, _ []string) error {
			if err := store.Ping(ctx); err != nil {
				return fmt.Errorf("database ping failed: %w", err)
			}
			fmt.Printf("Data directory: %s\n", cfg.DataDir)
			fmt.Printf("Database: OK\n")
			shards, _ := store.ShardRepo().ListAll(ctx)
			damaged, _ := store.ShardRepo().ListDamaged(ctx)
			fmt.Printf("Shards: %d total, %d damaged\n", len(shards), len(damaged))
			_, mailTotal, _ := store.MailItemRepo().List(ctx, storage.MailFilter{PageSize: 1})
			_, handTotal, _ := store.HandoverRepo().List(ctx, storage.HandoverFilter{PageSize: 1})
			_, dispTotal, _ := store.DispositionRepo().List(ctx, storage.DispositionFilter{PageSize: 1})
			_, ledgerTotal, _ := store.LedgerRepo().List(ctx, domain.LedgerQuery{PageSize: 1})
			fmt.Printf("Mail items: %d\n", mailTotal)
			fmt.Printf("Handover forms: %d\n", handTotal)
			fmt.Printf("Dispositions: %d\n", dispTotal)
			fmt.Printf("Ledger entries: %d\n", ledgerTotal)
			lastEventID, _ := store.EventRepo().GetLastID(ctx)
			fmt.Printf("Events: up to #%d\n", lastEventID)
			return nil
		}),
	}
}

func diagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose",
		Short: "Run detailed diagnostics on all subsystems",
		RunE: withStore(30*time.Second, func(ctx context.Context, store storage.Store, cfg config.Config, _ []string) error {
			maintSvc := service.NewMaintenanceService(store, domain.RealClock{})
			verify, err := maintSvc.Verify(ctx)
			if err != nil {
				return err
			}
			overdueSvc := service.NewOverdueService(store, domain.RealClock{}, cfg.SignoffTimeout())
			report, err := overdueSvc.GetOverdueReport(ctx)
			if err != nil {
				return err
			}
			failures, failTotal, _ := overdueSvc.ListFailures(ctx, domain.FailureStatusPermanent)
			fmt.Println("=== Diagnostic Report ===")
			fmt.Printf("Data integrity: %d OK shards, %d damaged\n", verify.OKShards, verify.DamagedShards)
			fmt.Printf("Overdue mails: %d\n", report.TotalOverdue)
			fmt.Printf("Backlog batches: %d\n", report.TotalBacklog)
			fmt.Printf("Permanent failures: %d\n", failTotal)
			if len(failures) > 0 {
				w := csv.NewWriter(os.Stdout)
				w.Write([]string{"id", "task_type", "entity_id", "last_error", "attempts"})
				for _, f := range failures {
					w.Write([]string{fmt.Sprintf("%d", f.ID), f.TaskType, f.EntityID, f.LastError, fmt.Sprintf("%d", f.Attempts)})
				}
				w.Flush()
			}
			return nil
		}),
	}
}
