package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/mailbox"
	"github.com/changmin/c4-core/internal/mcp/handlers/mailhandler"
)

func init() {
	registerInitHook(initMail)
	registerShutdownHook(shutdownMail)
}

// initMail opens the mailbox database and registers the c4_mail_* handlers.
func initMail(ctx *initContext) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: mail: cannot resolve home dir: %v\n", err)
		return nil
	}
	dbPath := filepath.Join(homeDir, ".c4", "mailbox.db")
	ms, err := mailbox.NewMailStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: mail store init failed: %v\n", err)
		return nil
	}
	ctx.mailStore = ms
	mailhandler.Register(ctx.reg, ms)
	return nil
}

// shutdownMail closes the mailbox database to flush WAL and prevent -wal/-shm residue.
func shutdownMail(ctx *initContext) {
	if ctx.mailStore != nil {
		ctx.mailStore.Close()
	}
}
