package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client provides a gRPC client for the EventBus daemon over Unix Domain Socket.
type Client struct {
	conn   *grpc.ClientConn
	client pb.EventBusClient
}

// NewClient dials the EventBus gRPC server at the given Unix socket path.
func NewClient(socketPath string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	target := "unix:" + socketPath
	conn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial eventbus unix:%s: %w", socketPath, err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewEventBusClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Publish sends an event to the EventBus daemon.
func (c *Client) Publish(evType, source string, data json.RawMessage, projectID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.client.Publish(ctx, &pb.Event{
		Type:        evType,
		Source:      source,
		Data:        data,
		ProjectId:   projectID,
		TimestampMs: time.Now().UnixMilli(),
	})
	if err != nil {
		return "", err
	}
	return resp.EventId, nil
}

// PublishAsync sends an event without waiting for confirmation (fire-and-forget).
// Errors are logged to stderr.
func (c *Client) PublishAsync(evType, source string, data json.RawMessage, projectID string) {
	go func() {
		if _, err := c.Publish(evType, source, data, projectID); err != nil {
			fmt.Fprintf(os.Stderr, "c4: eventbus: async publish %s: %v\n", evType, err)
		}
	}()
}

// ListEvents returns stored events with optional filters.
func (c *Client) ListEvents(evType string, limit int, sinceMs int64) ([]*pb.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.client.ListEvents(ctx, &pb.ListEventsRequest{
		Type:    evType,
		Limit:   int32(limit),
		SinceMs: sinceMs,
	})
	if err != nil {
		return nil, err
	}
	return resp.Events, nil
}

// Subscribe returns a channel of events matching the pattern.
// The caller should cancel the context to stop the subscription.
func (c *Client) Subscribe(ctx context.Context, pattern string, projectID string) (<-chan *pb.Event, error) {
	stream, err := c.client.Subscribe(ctx, &pb.SubscribeRequest{
		EventPattern: pattern,
		ProjectId:    projectID,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan *pb.Event, 64)
	go func() {
		defer close(ch)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// AddRule creates a new event routing rule.
func (c *Client) AddRule(name, pattern, filterJSON, actionType, actionConfig string, enabled bool, priority int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.client.AddRule(ctx, &pb.Rule{
		Name:         name,
		EventPattern: pattern,
		FilterJson:   filterJSON,
		ActionType:   actionType,
		ActionConfig: actionConfig,
		Enabled:      enabled,
		Priority:     int32(priority),
	})
	if err != nil {
		return "", err
	}
	return resp.RuleId, nil
}

// ListRules returns all configured rules.
func (c *Client) ListRules() ([]*pb.Rule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.client.ListRules(ctx, &pb.ListRulesRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Rules, nil
}

// RemoveRule removes a rule by ID or name.
func (c *Client) RemoveRule(id, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := c.client.RemoveRule(ctx, &pb.RemoveRuleRequest{
		Id:   id,
		Name: name,
	})
	return err
}

// ToggleRule enables or disables a rule by name.
func (c *Client) ToggleRule(name string, enabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := c.client.ToggleRule(ctx, &pb.ToggleRuleRequest{
		Name:    name,
		Enabled: enabled,
	})
	return err
}

// ListLogs returns dispatch log entries with optional filters.
// Optional eventType filters by event type (passed as first variadic arg).
func (c *Client) ListLogs(eventID string, limit int, sinceMs int64, eventType ...string) ([]*pb.LogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.ListLogsRequest{
		EventId: eventID,
		Limit:   int32(limit),
		SinceMs: sinceMs,
	}
	if len(eventType) > 0 && eventType[0] != "" {
		req.EventType = eventType[0]
	}

	resp, err := c.client.ListLogs(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Logs, nil
}

// GetStats returns aggregate eventbus statistics.
func (c *Client) GetStats() (*pb.GetStatsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return c.client.GetStats(ctx, &pb.GetStatsRequest{})
}

// ReplayEvents streams stored events, optionally re-dispatching them.
func (c *Client) ReplayEvents(ctx context.Context, evType string, sinceMs int64, limit int, dryRun bool) (<-chan *pb.Event, error) {
	stream, err := c.client.ReplayEvents(ctx, &pb.ReplayRequest{
		EventType: evType,
		SinceMs:   sinceMs,
		Limit:     int32(limit),
		DryRun:    dryRun,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan *pb.Event, 64)
	go func() {
		defer close(ch)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
