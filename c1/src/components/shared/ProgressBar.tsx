import type { TaskProgress } from '../../types';

interface ProgressBarProps {
  progress: TaskProgress;
}

export function ProgressBar({ progress }: ProgressBarProps) {
  const total = (progress.done + progress.in_progress + progress.pending + progress.blocked) || 1;
  const donePercent = (progress.done / total) * 100;
  const inProgressPercent = (progress.in_progress / total) * 100;
  const blockedPercent = (progress.blocked / total) * 100;
  const pendingPercent = (progress.pending / total) * 100;

  return (
    <div className="progress-bar">
      <div className="progress-bar__track">
        <div
          className="progress-bar__segment progress-bar__segment--done"
          style={{ width: `${donePercent}%` }}
        />
        <div
          className="progress-bar__segment progress-bar__segment--in-progress"
          style={{ width: `${inProgressPercent}%` }}
        />
        {blockedPercent > 0 && (
          <div
            className="progress-bar__segment progress-bar__segment--blocked"
            style={{ width: `${blockedPercent}%` }}
          />
        )}
        <div
          className="progress-bar__segment progress-bar__segment--pending"
          style={{ width: `${pendingPercent}%` }}
        />
      </div>
      <div className="progress-bar__labels">
        <span className="progress-bar__label progress-bar__label--done">
          {progress.done} done
        </span>
        <span className="progress-bar__label progress-bar__label--in-progress">
          {progress.in_progress} active
        </span>
        {progress.blocked > 0 && (
          <span className="progress-bar__label progress-bar__label--blocked">
            {progress.blocked} blocked
          </span>
        )}
        <span className="progress-bar__label progress-bar__label--pending">
          {progress.pending} pending
        </span>
      </div>
    </div>
  );
}
