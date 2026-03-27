//go:build hub

package main

import (
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp/handlers/relayhandler"
)

// hubListerAdapter wraps hub.Client to satisfy relayhandler.HubWorkerLister.
type hubListerAdapter struct {
	client *hub.Client
}

func (a *hubListerAdapter) ListWorkersBasic() ([]relayhandler.HubListerResult, int, error) {
	workers, pending, err := a.client.ListWorkersBasic()
	if err != nil {
		return nil, 0, err
	}
	result := make([]relayhandler.HubListerResult, len(workers))
	for i, w := range workers {
		result[i] = relayhandler.HubListerResult{
			ID:       w.ID,
			Hostname: w.Hostname,
			Status:   w.Status,
			Tags:     w.Tags,
			GPUModel: w.GPUModel,
		}
	}
	return result, pending, nil
}

func newHubListerAdapter(hubClientAny any) relayhandler.HubWorkerLister {
	hc, ok := hubClientAny.(*hub.Client)
	if !ok || hc == nil {
		return nil
	}
	return &hubListerAdapter{client: hc}
}
