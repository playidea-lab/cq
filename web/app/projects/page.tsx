import Header from '../components/Header';
import ProjectCard from '../components/ProjectCard';
import { Project } from '@/lib/api';

// Mock data for now - will be replaced with API call
const mockProjects: Project[] = [
  {
    id: 'c4',
    name: 'C4 Core',
    status: 'EXECUTE',
    tasks_done: 47,
    tasks_pending: 22,
    created_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'demo-app',
    name: 'Demo Application',
    status: 'PLAN',
    tasks_done: 0,
    tasks_pending: 15,
    created_at: '2024-01-15T00:00:00Z',
  },
];

export default function ProjectsPage() {
  // In production, this would be fetched from API
  const projects = mockProjects;

  return (
    <div className="flex flex-col min-h-screen bg-gray-900">
      <Header />
      <main className="flex-1 max-w-6xl w-full mx-auto p-8">
        <div className="flex items-center justify-between mb-8">
          <h1 className="text-2xl font-bold text-white">Projects</h1>
          <button className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700 transition-colors">
            New Project
          </button>
        </div>

        {projects.length === 0 ? (
          <div className="text-center py-16">
            <p className="text-gray-400 mb-4">No projects yet</p>
            <button className="bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700">
              Create your first project
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {projects.map((project) => (
              <ProjectCard key={project.id} project={project} />
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
