package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply or roll back database migrations",
	}
	cmd.AddCommand(
		newMigrateUpCmd(),
		newMigrateDownCmd(),
		newMigrateStepsCmd(),
		newMigrateVersionCmd(),
		newMigrateForceCmd(),
	)
	return cmd
}

func loadMigrator(cmd *cobra.Command) (*db.Migrator, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := cfg.RequireDatabase(); err != nil {
		return nil, err
	}
	m, err := db.NewMigrator(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	// Best-effort close on command completion via PostRunE handled by caller.
	cmd.PostRunE = func(*cobra.Command, []string) error { return m.Close() }
	return m, nil
}

func newMigrateUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := loadMigrator(cmd)
			if err != nil {
				return err
			}
			if err := m.Up(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "migrations applied")
			return nil
		},
	}
}

func newMigrateDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Roll back all migrations (destructive)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := loadMigrator(cmd)
			if err != nil {
				return err
			}
			if err := m.Down(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "migrations rolled back")
			return nil
		},
	}
}

func newMigrateStepsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "steps <n>",
		Short: "Apply N migrations forward (positive) or backward (negative)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid step count %q: %w", args[0], err)
			}
			m, err := loadMigrator(cmd)
			if err != nil {
				return err
			}
			if err := m.Steps(n); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "applied %d step(s)\n", n)
			return nil
		},
	}
}

func newMigrateVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the current migration version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := loadMigrator(cmd)
			if err != nil {
				return err
			}
			v, dirty, err := m.Version()
			if err != nil {
				return err
			}
			if v == 0 && !dirty {
				fmt.Fprintln(cmd.OutOrStdout(), "no migrations applied")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "version: %d (dirty=%t)\n", v, dirty)
			return nil
		},
	}
}

func newMigrateForceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "force <version>",
		Short: "Force the schema version (recovers from a dirty state)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid version %q: %w", args[0], err)
			}
			m, err := loadMigrator(cmd)
			if err != nil {
				return err
			}
			if err := m.Force(v); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "forced version: %d\n", v)
			return nil
		},
	}
}
