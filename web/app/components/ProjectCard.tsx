import Link from 'next/link';
import { Project } from '@/lib/api';

interface ProjectCardProps {
  project: Project;
}

export default function ProjectCard({ project }: ProjectCardProps) {
  const progress = project.tasks_done + project.tasks_pending > 0
    ? Math.round((project.tasks_done / (project.tasks_done + project.tasks_pending)) * 100)
    : 0;

  const statusColors: Record<string, string> = {
    PLAN: 'bg-yellow-500',
    EXECUTE: 'bg-blue-500',
    VERIFY: 'bg-purple-500',
    COMPLETE: 'bg-green-500',
    STOPPED: 'bg-gray-500',
  };

  return (
    <Link href={`/projects/${project.id}`}>
      <div className="bg-gray-800 rounded-lg p-6 hover:bg-gray-750 transition-colors border border-gray-700 hover:border-gray-600">
        <div className="flex items-start justify-between mb-4">
          <h3 className="text-lg font-semibold text-white">{project.name}</h3>
          <span className={`px-2 py-1 rounded text-xs font-medium ${statusColors[project.status] || 'bg-gray-500'}`}>
            {project.status}
          </span>
        </div>

        {/* Progress Bar */}
        <div className="mb-4">
          <div className="flex justify-between text-sm text-gray-400 mb-1">
            <span>Progress</span>
            <span>{progress}%</span>
          </div>
          <div className="h-2 bg-gray-700 rounded-full overflow-hidden">
            <div
              className="h-full bg-blue-500 rounded-full transition-all"
              style={{ width: `${progress}%` }}
            />
          </div>
        </div>

        {/* Stats */}
        <div className="flex gap-4 text-sm">
          <div>
            <span className="text-gray-400">Done: </span>
            <span className="text-green-400 font-medium">{project.tasks_done}</span>
          </div>
          <div>
            <span className="text-gray-400">Pending: </span>
            <span className="text-yellow-400 font-medium">{project.tasks_pending}</span>
          </div>
        </div>

        {/* Created Date */}
        <p className="text-xs text-gray-500 mt-4">
          Created {new Date(project.created_at).toLocaleDateString()}
        </p>
      </div>
    </Link>
  );
}
