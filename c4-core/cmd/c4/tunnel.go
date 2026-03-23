package main

import (
	"fmt"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/relay"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Relay tunnel commands",
	Long: `Commands for relay-based data tunnels.

Subcommands:
  receive  - Receive a tar stream from the relay server and extract it locally`,
}

var tunnelReceiveDest string

var tunnelReceiveCmd = &cobra.Command{
	Use:   "receive <tunnel-id>",
	Short: "Receive a tar stream via relay tunnel and extract to dest",
	Long: `Connect to the relay server as a tunnel receiver and extract incoming tar data.

The relay URL is read from .c4/config.yaml (relay.url).
The auth token is read from the saved session.

Example:
  cq tunnel receive abc123 --dest /data/received`,
	Args: cobra.ExactArgs(1),
	RunE: runTunnelReceive,
}

func init() {
	tunnelReceiveCmd.Flags().StringVar(&tunnelReceiveDest, "dest", ".", "destination directory for extracted files")
	tunnelCmd.AddCommand(tunnelReceiveCmd)
	rootCmd.AddCommand(tunnelCmd)
}

func runTunnelReceive(cmd *cobra.Command, args []string) error {
	tunnelID := args[0]

	// Load relay URL from config.
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg := cfgMgr.GetConfig()
	relayURL := cfg.Relay.URL
	if relayURL == "" {
		return fmt.Errorf("relay.url not configured in .c4/config.yaml")
	}

	// Load auth token from saved session.
	token := ""
	authClient, err := newAuthClient()
	if err == nil {
		if session, err := authClient.GetSession(); err == nil && session != nil {
			token = session.AccessToken
		}
	}

	fmt.Printf("Connecting to relay tunnel %s (dest: %s)...\n", tunnelID, tunnelReceiveDest)
	if err := relay.HandleTunnel(relayURL, tunnelID, tunnelReceiveDest, token); err != nil {
		return fmt.Errorf("tunnel receive: %w", err)
	}
	fmt.Println("Tunnel receive complete.")
	return nil
}
