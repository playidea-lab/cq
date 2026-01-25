'use client';

import Link from 'next/link';
import { Team, TeamPlan, getPlanDisplayName } from '@/lib/teams';

interface TeamCardProps {
  team: Team;
  memberCount?: number;
}

export default function TeamCard({ team, memberCount = 0 }: TeamCardProps) {
  const planColors: Record<TeamPlan, string> = {
    [TeamPlan.FREE]: 'bg-gray-500',
    [TeamPlan.PRO]: 'bg-blue-500',
    [TeamPlan.TEAM]: 'bg-purple-500',
    [TeamPlan.AGENCY]: 'bg-orange-500',
    [TeamPlan.ENTERPRISE]: 'bg-yellow-500',
  };

  return (
    <Link href={`/teams/${team.id}`}>
      <div className="bg-gray-800 rounded-lg p-6 hover:bg-gray-750 transition-colors border border-gray-700 hover:border-gray-600">
        <div className="flex items-start justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold text-white">{team.name}</h3>
            <p className="text-sm text-gray-400">/{team.slug}</p>
          </div>
          <span
            className={`px-2 py-1 rounded text-xs font-medium text-white ${
              planColors[team.plan] || 'bg-gray-500'
            }`}
          >
            {getPlanDisplayName(team.plan)}
          </span>
        </div>

        {/* Member Count */}
        <div className="flex items-center gap-2 text-sm text-gray-400 mb-4">
          <svg
            className="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
            />
          </svg>
          <span>{memberCount} members</span>
        </div>

        {/* Settings Preview */}
        {team.settings && Object.keys(team.settings).length > 0 && (
          <div className="flex flex-wrap gap-2 mb-4">
            {Object.entries(team.settings).slice(0, 3).map(([key]) => (
              <span
                key={key}
                className="px-2 py-1 bg-gray-700 text-gray-300 rounded text-xs"
              >
                {key}
              </span>
            ))}
          </div>
        )}

        {/* Created Date */}
        <p className="text-xs text-gray-500">
          Created {new Date(team.created_at).toLocaleDateString()}
        </p>
      </div>
    </Link>
  );
}
