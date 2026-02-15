import { useState, useMemo } from 'react';
import { useEventBus, type EventBusEvent } from '../../hooks/useEventBus';
import './eventbus.css';

function EventStream({
  events,
  selectedId,
  onSelect,
}: {
  events: EventBusEvent[];
  selectedId: string | null;
  onSelect: (event: EventBusEvent) => void;
}) {
  if (events.length === 0) {
    return (
      <div className="eventbus-stream__empty">
        Waiting for events...
      </div>
    );
  }

  return (
    <div className="eventbus-stream">
      {events.map((ev) => (
        <button
          key={ev.id}
          className={`eventbus-stream__item ${selectedId === ev.id ? 'eventbus-stream__item--selected' : ''}`}
          onClick={() => onSelect(ev)}
        >
          <span className="eventbus-stream__time">
            {new Date(ev.timestamp_ms).toLocaleTimeString()}
          </span>
          <span className="eventbus-stream__type">{ev.type}</span>
          <span className="eventbus-stream__source">{ev.source}</span>
          {ev.correlation_id && (
            <span className="eventbus-stream__corr" title={ev.correlation_id}>
              {ev.correlation_id.slice(0, 8)}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}

function EventDetail({ event }: { event: EventBusEvent | null }) {
  if (!event) {
    return (
      <div className="eventbus-detail__empty">
        Select an event to view details
      </div>
    );
  }

  return (
    <div className="eventbus-detail">
      <div className="eventbus-detail__header">
        <h3>{event.type}</h3>
        <span className="eventbus-detail__id">{event.id}</span>
      </div>
      <dl className="eventbus-detail__meta">
        <dt>Source</dt>
        <dd>{event.source}</dd>
        <dt>Time</dt>
        <dd>{new Date(event.timestamp_ms).toISOString()}</dd>
        {event.project_id && (
          <>
            <dt>Project</dt>
            <dd>{event.project_id}</dd>
          </>
        )}
        {event.correlation_id && (
          <>
            <dt>Correlation</dt>
            <dd>{event.correlation_id}</dd>
          </>
        )}
      </dl>
      <div className="eventbus-detail__data">
        <h4>Data</h4>
        <pre>{JSON.stringify(event.data, null, 2)}</pre>
      </div>
    </div>
  );
}

function StatsPanel({ events }: { events: EventBusEvent[] }) {
  const stats = useMemo(() => {
    const byType: Record<string, number> = {};
    for (const ev of events) {
      byType[ev.type] = (byType[ev.type] || 0) + 1;
    }
    return Object.entries(byType)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 10);
  }, [events]);

  if (stats.length === 0) return null;

  return (
    <div className="eventbus-stats">
      <h4>Event Types</h4>
      <ul>
        {stats.map(([type, count]) => (
          <li key={type}>
            <span className="eventbus-stats__type">{type}</span>
            <span className="eventbus-stats__count">{count}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function EventBusView() {
  const { events, status, isConnected, paused, setPaused, connect, disconnect, clear } = useEventBus();
  const [selectedEvent, setSelectedEvent] = useState<EventBusEvent | null>(null);
  const [filterText, setFilterText] = useState('');

  const filteredEvents = useMemo(() => {
    if (!filterText) return events;
    const lower = filterText.toLowerCase();
    return events.filter(
      (ev) =>
        ev.type.toLowerCase().includes(lower) ||
        ev.source.toLowerCase().includes(lower) ||
        (ev.correlation_id && ev.correlation_id.toLowerCase().includes(lower))
    );
  }, [events, filterText]);

  return (
    <div className="eventbus-view">
      <div className="eventbus-toolbar">
        <div className="eventbus-toolbar__left">
          <span className={`eventbus-toolbar__status eventbus-toolbar__status--${status}`}>
            {status}
          </span>
          {!isConnected ? (
            <button className="btn btn--primary btn--sm" onClick={connect}>Connect</button>
          ) : (
            <button className="btn btn--secondary btn--sm" onClick={disconnect}>Disconnect</button>
          )}
          <button
            className={`btn btn--sm ${paused ? 'btn--primary' : 'btn--secondary'}`}
            onClick={() => setPaused(!paused)}
          >
            {paused ? 'Resume' : 'Pause'}
          </button>
          <button className="btn btn--secondary btn--sm" onClick={clear}>Clear</button>
        </div>
        <div className="eventbus-toolbar__right">
          <input
            type="text"
            className="eventbus-toolbar__filter"
            placeholder="Filter events..."
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
          />
          <span className="eventbus-toolbar__count">{filteredEvents.length} events</span>
        </div>
      </div>

      <div className="eventbus-content">
        <div className="eventbus-content__stream">
          <EventStream
            events={filteredEvents}
            selectedId={selectedEvent?.id ?? null}
            onSelect={setSelectedEvent}
          />
        </div>
        <div className="eventbus-content__detail">
          <EventDetail event={selectedEvent} />
          <StatsPanel events={events} />
        </div>
      </div>
    </div>
  );
}
