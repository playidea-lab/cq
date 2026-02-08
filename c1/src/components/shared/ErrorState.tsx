interface ErrorStateProps {
  message: string;
  detail?: string;
  onRetry?: () => void;
}

export function ErrorState({ message, detail, onRetry }: ErrorStateProps) {
  return (
    <div className="error-state" role="alert">
      <div className="error-state__icon">\u26A0</div>
      <p className="error-state__message">{message}</p>
      {detail && <p className="error-state__detail">{detail}</p>}
      {onRetry && (
        <button className="btn btn--secondary btn--sm" onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  );
}
