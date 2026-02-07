import type { TaskProgress } from '../../types';

interface ProgressBarProps {
  progress: TaskProgress;
}

export function ProgressBar({ progress }: ProgressBarProps) {
  const total = progress.total || 1;
  const donePercent = (progress.done / total) * 100;
  const inProgressPercent = (progress.in_progress / total) * 100;
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
        <span className="progress-bar__label progress-bar__label--pending">
          {progress.pending} pending
        </span>
      </div>
    </div>
  );
}
