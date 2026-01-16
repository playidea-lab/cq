import Header from '../../components/Header';
import Chat from '../../components/Chat';

interface ProjectDetailProps {
  params: Promise<{ id: string }>;
}

export default async function ProjectDetailPage({ params }: ProjectDetailProps) {
  const { id } = await params;
  
  // Mock data - will be fetched from API
  const project = {
    id,
    name: id === 'c4' ? 'C4 Core' : 'Project ' + id,
    status: 'EXECUTE',
    tasks_done: 47,
    tasks_pending: 22,
    description: 'AI Project Orchestration System',
    workers: {
      'worker-main': { state: 'idle', task_id: null },
      'worker-7f2a9c1e': { state: 'working', task_id: 'T-503' },
    },
    recent_events: [
      { type: 'task_completed', timestamp: new Date().toISOString(), data: { task_id: 'T-502' } },
      { type: 'task_started', timestamp: new Date().toISOString(), data: { task_id: 'T-503' } },
    ],
  };

  const progress = Math.round(
    (project.tasks_done / (project.tasks_done + project.tasks_pending)) * 100
  );

  return (
    <div className="flex flex-col min-h-screen bg-gray-900">
      <Header />
      <main className="flex-1 max-w-7xl w-full mx-auto p-8">
        {/* Project Header */}
        <div className="bg-gray-800 rounded-lg p-6 mb-6 border border-gray-700">
          <div className="flex items-start justify-between">
            <div>
              <h1 className="text-2xl font-bold text-white mb-2">{project.name}</h1>
              <p className="text-gray-400">{project.description}</p>
            </div>
            <span className="px-3 py-1 bg-blue-500 text-white rounded text-sm font-medium">
              {project.status}
            </span>
          </div>

          {/* Progress */}
          <div className="mt-6">
            <div className="flex justify-between text-sm text-gray-400 mb-2">
              <span>Overall Progress</span>
              <span>{progress}% ({project.tasks_done}/{project.tasks_done + project.tasks_pending})</span>
            </div>
            <div className="h-3 bg-gray-700 rounded-full overflow-hidden">
              <div
                className="h-full bg-blue-500 rounded-full"
                style={{ width: `${progress}%` }}
              />
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Workers */}
          <div className="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <h2 className="text-lg font-semibold text-white mb-4">Workers</h2>
            <div className="space-y-3">
              {Object.entries(project.workers).map(([workerId, worker]) => (
                <div
                  key={workerId}
                  className="flex items-center justify-between p-3 bg-gray-700 rounded-lg"
                >
                  <div>
                    <p className="text-sm font-medium text-white">{workerId}</p>
                    {worker.task_id && (
                      <p className="text-xs text-gray-400">{worker.task_id}</p>
                    )}
                  </div>
                  <span
                    className={`w-2 h-2 rounded-full ${
                      worker.state === 'working' ? 'bg-green-500' : 'bg-gray-500'
                    }`}
                  />
                </div>
              ))}
            </div>
          </div>

          {/* Recent Events */}
          <div className="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <h2 className="text-lg font-semibold text-white mb-4">Recent Events</h2>
            <div className="space-y-3">
              {project.recent_events.map((event, i) => (
                <div key={i} className="p-3 bg-gray-700 rounded-lg">
                  <p className="text-sm font-medium text-white">{event.type}</p>
                  <p className="text-xs text-gray-400">
                    {new Date(event.timestamp).toLocaleTimeString()}
                  </p>
                </div>
              ))}
            </div>
          </div>

          {/* Quick Actions */}
          <div className="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <h2 className="text-lg font-semibold text-white mb-4">Actions</h2>
            <div className="space-y-2">
              <button className="w-full bg-blue-600 text-white py-2 rounded-lg hover:bg-blue-700 transition-colors">
                Start Task
              </button>
              <button className="w-full bg-gray-600 text-white py-2 rounded-lg hover:bg-gray-500 transition-colors">
                View Tasks
              </button>
              <button className="w-full bg-gray-600 text-white py-2 rounded-lg hover:bg-gray-500 transition-colors">
                Create Checkpoint
              </button>
            </div>
          </div>
        </div>

        {/* Project Chat */}
        <div className="mt-6 bg-gray-800 rounded-lg border border-gray-700 h-96">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Project Chat</h2>
          </div>
          <div className="h-80">
            <Chat conversationId={`project-${id}`} />
          </div>
        </div>
      </main>
    </div>
  );
}
