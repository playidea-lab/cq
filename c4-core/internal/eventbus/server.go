package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// warnf is the package-level warning logger used by the EventBus server.
// It may be overridden in tests for log capture.
var warnf = log.Printf

// ServerConfig holds configuration for the gRPC EventBus server.
type ServerConfig struct {
	Store      *Store
	Dispatcher *Dispatcher
}

// Server implements the gRPC EventBus service.
type Server struct {
	pb.UnimplementedEventBusServer
	store      *Store
	dispatcher *Dispatcher

	// Subscriber management
	mu          sync.RWMutex
	subscribers map[string][]chan *pb.Event // pattern -> channels
}

// NewServer creates a new EventBus gRPC server.
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		store:       cfg.Store,
		dispatcher:  cfg.Dispatcher,
		subscribers: make(map[string][]chan *pb.Event),
	}
}

// Publish stores an event and dispatches it to matched rules and subscribers.
func (s *Server) Publish(ctx context.Context, req *pb.Event) (*pb.PublishResponse, error) {
	if req.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "event type is required")
	}

	if req.Source == "" {
		req.Source = "unknown"
	}
	if req.TimestampMs == 0 {
		req.TimestampMs = time.Now().UnixMilli()
	}

	data := json.RawMessage(req.Data)
	if len(data) == 0 {
		data = json.RawMessage("{}")
	}

	eventID, err := s.store.StoreEvent(req.Type, req.Source, data, req.ProjectId, req.CorrelationId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "store event: %v", err)
	}

	// Async dispatch to rules
	s.dispatcher.Dispatch(eventID, req.Type, data)

	// Notify subscribers
	s.notifySubscribers(req.Type, &pb.Event{
		Id:            eventID,
		Type:          req.Type,
		Source:        req.Source,
		Data:          req.Data,
		ProjectId:     req.ProjectId,
		TimestampMs:   req.TimestampMs,
		CorrelationId: req.CorrelationId,
	})

	return &pb.PublishResponse{
		EventId: eventID,
		Ok:      true,
	}, nil
}

// Subscribe streams events matching the given pattern to the client.
func (s *Server) Subscribe(req *pb.SubscribeRequest, stream pb.EventBus_SubscribeServer) error {
	pattern := req.EventPattern
	if pattern == "" {
		pattern = "*"
	}

	if req.ProjectId == "" {
		warnf("[eventbus] WARN: subscribe without project_id (pattern=%s). Use SubscribeWithProject.", pattern)
	}

	ch := make(chan *pb.Event, 64)
	s.addSubscriber(pattern, ch)
	defer s.removeSubscriber(pattern, ch)

	for {
		select {
		case ev := <-ch:
			// Filter by project_id if specified
			if req.ProjectId != "" && ev.ProjectId != req.ProjectId {
				continue
			}
			if err := stream.Send(ev); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// ListEvents returns stored events with optional filters.
func (s *Server) ListEvents(ctx context.Context, req *pb.ListEventsRequest) (*pb.ListEventsResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	events, err := s.store.ListEvents(req.Type, limit, req.SinceMs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list events: %v", err)
	}

	pbEvents := make([]*pb.Event, 0, len(events))
	for _, e := range events {
		pbEvents = append(pbEvents, &pb.Event{
			Id:            e.ID,
			Type:          e.Type,
			Source:        e.Source,
			Data:          []byte(e.Data),
			ProjectId:     e.ProjectID,
			CorrelationId: e.CorrelationID,
			TimestampMs:   e.CreatedAt.UnixMilli(),
		})
	}

	return &pb.ListEventsResponse{
		Events: pbEvents,
		Total:  int32(len(pbEvents)),
	}, nil
}

// AddRule creates a new event routing rule.
func (s *Server) AddRule(ctx context.Context, req *pb.Rule) (*pb.RuleResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "rule name is required")
	}
	if req.EventPattern == "" {
		return nil, status.Error(codes.InvalidArgument, "event_pattern is required")
	}
	if req.ActionType == "" {
		return nil, status.Error(codes.InvalidArgument, "action_type is required")
	}

	id, err := s.store.AddRule(req.Name, req.EventPattern, req.FilterJson, req.ActionType, req.ActionConfig, req.Enabled, int(req.Priority))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "add rule: %v", err)
	}

	return &pb.RuleResponse{
		RuleId: id,
		Ok:     true,
	}, nil
}

// ListRules returns all configured rules.
func (s *Server) ListRules(ctx context.Context, req *pb.ListRulesRequest) (*pb.ListRulesResponse, error) {
	rules, err := s.store.ListRules()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list rules: %v", err)
	}

	pbRules := make([]*pb.Rule, 0, len(rules))
	for _, r := range rules {
		pbRules = append(pbRules, &pb.Rule{
			Id:           r.ID,
			Name:         r.Name,
			EventPattern: r.EventPattern,
			FilterJson:   r.FilterJSON,
			ActionType:   r.ActionType,
			ActionConfig: r.ActionConfig,
			Enabled:      r.Enabled,
			Priority:     int32(r.Priority),
		})
	}

	return &pb.ListRulesResponse{Rules: pbRules}, nil
}

// RemoveRule deletes a rule by ID or name.
func (s *Server) RemoveRule(ctx context.Context, req *pb.RemoveRuleRequest) (*pb.RuleResponse, error) {
	if req.Id == "" && req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "id or name is required")
	}

	if err := s.store.RemoveRule(req.Id, req.Name); err != nil {
		return nil, status.Errorf(codes.NotFound, "remove rule: %v", err)
	}

	return &pb.RuleResponse{Ok: true}, nil
}

// ToggleRule enables or disables a rule by name.
func (s *Server) ToggleRule(ctx context.Context, req *pb.ToggleRuleRequest) (*pb.RuleResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "rule name is required")
	}

	if err := s.store.ToggleRule(req.Name, req.Enabled); err != nil {
		return nil, status.Errorf(codes.NotFound, "toggle rule: %v", err)
	}

	return &pb.RuleResponse{Ok: true}, nil
}

// ListLogs returns dispatch log entries with optional filters.
func (s *Server) ListLogs(ctx context.Context, req *pb.ListLogsRequest) (*pb.ListLogsResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	logs, err := s.store.ListLogs(req.EventId, limit, req.SinceMs, req.EventType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list logs: %v", err)
	}

	pbLogs := make([]*pb.LogEntry, 0, len(logs))
	for _, l := range logs {
		pbLogs = append(pbLogs, &pb.LogEntry{
			Id:          l.ID,
			EventId:     l.EventID,
			RuleName:    l.RuleName,
			EventType:   l.EventType,
			Status:      l.Status,
			Error:       l.Error,
			DurationMs:  l.DurationMs,
			TimestampMs: l.CreatedAt.UnixMilli(),
		})
	}

	return &pb.ListLogsResponse{
		Logs:  pbLogs,
		Total: int32(len(pbLogs)),
	}, nil
}

// GetStats returns aggregate statistics about the eventbus.
func (s *Server) GetStats(ctx context.Context, req *pb.GetStatsRequest) (*pb.GetStatsResponse, error) {
	stats, err := s.store.EventStats()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get stats: %v", err)
	}

	resp := &pb.GetStatsResponse{}
	if v, ok := stats["event_count"].(int); ok {
		resp.EventCount = int32(v)
	}
	if v, ok := stats["rule_count"].(int); ok {
		resp.RuleCount = int32(v)
	}
	if v, ok := stats["log_count"].(int); ok {
		resp.LogCount = int32(v)
	}
	if v, ok := stats["oldest_event"].(string); ok {
		resp.OldestEvent = v
	}
	if v, ok := stats["newest_event"].(string); ok {
		resp.NewestEvent = v
	}

	return resp, nil
}

// ReplayEvents streams stored events and optionally re-dispatches them.
func (s *Server) ReplayEvents(req *pb.ReplayRequest, stream pb.EventBus_ReplayEventsServer) error {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}

	events, err := s.store.ListEventsASC(req.EventType, req.SinceMs, limit)
	if err != nil {
		return status.Errorf(codes.Internal, "list events for replay: %v", err)
	}

	for _, e := range events {
		pbEvent := &pb.Event{
			Id:            e.ID,
			Type:          e.Type,
			Source:        e.Source,
			Data:          []byte(e.Data),
			ProjectId:     e.ProjectID,
			CorrelationId: e.CorrelationID,
			TimestampMs:   e.CreatedAt.UnixMilli(),
		}

		if err := stream.Send(pbEvent); err != nil {
			return err
		}

		// Re-dispatch if not dry_run
		if !req.DryRun && s.dispatcher != nil {
			s.dispatcher.DispatchSync(e.ID, e.Type, e.Data)
		}
	}

	return nil
}

// ListDLQ returns dead letter queue entries.
func (s *Server) ListDLQ(ctx context.Context, req *pb.ListDLQRequest) (*pb.ListDLQResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	entries, err := s.store.ListDLQ(limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list dlq: %v", err)
	}

	pbEntries := make([]*pb.DLQEntry, 0, len(entries))
	for _, e := range entries {
		pbEntries = append(pbEntries, &pb.DLQEntry{
			Id:          e.ID,
			EventId:     e.EventID,
			RuleId:      e.RuleID,
			RuleName:    e.RuleName,
			EventType:   e.EventType,
			Error:       e.Error,
			RetryCount:  int32(e.RetryCount),
			MaxRetries:  int32(e.MaxRetries),
			CreatedAtMs: e.CreatedAt.UnixMilli(),
		})
	}

	return &pb.ListDLQResponse{
		Entries: pbEntries,
		Total:   int32(len(pbEntries)),
	}, nil
}

func (s *Server) addSubscriber(pattern string, ch chan *pb.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers[pattern] = append(s.subscribers[pattern], ch)
}

func (s *Server) removeSubscriber(pattern string, ch chan *pb.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := s.subscribers[pattern]
	for i, c := range subs {
		if c == ch {
			s.subscribers[pattern] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
}

func (s *Server) notifySubscribers(eventType string, ev *pb.Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for pattern, subs := range s.subscribers {
		if matchPattern(pattern, eventType) {
			for _, ch := range subs {
				select {
				case ch <- ev:
				default:
					fmt.Fprintf(os.Stderr, "c4: eventbus: subscriber channel full, dropping event\n")
				}
			}
		}
	}
}
