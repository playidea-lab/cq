//go:build !hub

package main

import "github.com/changmin/c4-core/internal/mcp/handlers/relayhandler"

func newHubListerAdapter(_ any) relayhandler.HubWorkerLister {
	return nil
}
