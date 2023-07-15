// This package provides CLI interface for the backup package in module root.
package main

import (
	"bufio"
	"log"
	"os"

	"github.com/alecthomas/kong"
	backup "github.com/borsosl/go-local-backup"
)

// This is the CLI entry point of the backup application.
// See documentation at https://github.com/borsosl/go-local-backup/README.md
func main() {
	var args struct {
		Config string `arg:"" help:"Path to configuration"`
		DryRun bool   `short:"d" help:"Lists affected files without copying"`
	}
	kong.Parse(&args)

	config, err := os.Open(args.Config)
	if err != nil {
		log.Fatal("Cannot read config file: ", err)
	}
	defer config.Close()

	var cfg []string
	scanner := bufio.NewScanner(config)
	for scanner.Scan() {
		cfg = append(cfg, scanner.Text())
	}

	err = backup.Backup(cfg, os.Stdout, args.DryRun)
	if err != nil {
		os.Exit(1)
	}
}
