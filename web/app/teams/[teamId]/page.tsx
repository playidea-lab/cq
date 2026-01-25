'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import Header from '../../components/Header';
import {
  Team,
  TeamMember,
  TeamPlan,
  TeamRole,
  getTeam,
  listMembers,
  getPlanDisplayName,
  getRoleDisplayName,
} from '@/lib/teams';

export default function TeamDetailPage() {
  const params = useParams();
  const teamId = params.teamId as string;

  const [team, setTeam] = useState<Team | null>(null);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchData() {
      try {
        const [teamData, membersData] = await Promise.all([
          getTeam(teamId),
          listMembers(teamId),
        ]);
        setTeam(teamData);
        setMembers(membersData);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load team');
      } finally {
        setLoading(false);
      }
    }

    fetchData();
  }, [teamId]);

  if (loading) {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <Header />
        <main className="flex-1 flex items-center justify-center">
          <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-blue-500" />
        </main>
      </div>
    );
  }

  if (error || !team) {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <Header />
        <main className="flex-1 flex items-center justify-center">
          <div className="text-center">
            <p className="text-red-400 mb-4">{error || 'Team not found'}</p>
            <Link href="/teams" className="text-blue-400 hover:text-blue-300">
              Back to Teams
            </Link>
          </div>
        </main>
      </div>
    );
  }

  const planColors: Record<TeamPlan, string> = {
    [TeamPlan.FREE]: 'bg-gray-500',
    [TeamPlan.PRO]: 'bg-blue-500',
    [TeamPlan.TEAM]: 'bg-purple-500',
    [TeamPlan.AGENCY]: 'bg-orange-500',
    [TeamPlan.ENTERPRISE]: 'bg-yellow-500',
  };

  return (
    <div className="flex flex-col min-h-screen bg-gray-900">
      <Header />
      <main className="flex-1 max-w-6xl w-full mx-auto p-8">
        {/* Breadcrumb */}
        <div className="flex items-center gap-2 text-sm text-gray-400 mb-6">
          <Link href="/teams" className="hover:text-white">
            Teams
          </Link>
          <span>/</span>
          <span className="text-white">{team.name}</span>
        </div>

        {/* Team Header */}
        <div className="bg-gray-800 rounded-lg p-6 border border-gray-700 mb-8">
          <div className="flex items-start justify-between">
            <div>
              <div className="flex items-center gap-3 mb-2">
                <h1 className="text-2xl font-bold text-white">{team.name}</h1>
                <span
                  className={`px-2 py-1 rounded text-xs font-medium text-white ${
                    planColors[team.plan]
                  }`}
                >
                  {getPlanDisplayName(team.plan)}
                </span>
              </div>
              <p className="text-gray-400">/{team.slug}</p>
            </div>
            <Link
              href={`/teams/${teamId}/settings`}
              className="px-4 py-2 border border-gray-600 text-gray-300 rounded-lg hover:bg-gray-700 transition-colors"
            >
              Settings
            </Link>
          </div>

          {/* Quick Stats */}
          <div className="grid grid-cols-3 gap-6 mt-6 pt-6 border-t border-gray-700">
            <div>
              <p className="text-sm text-gray-400">Members</p>
              <p className="text-2xl font-semibold text-white">
                {members.length}
              </p>
            </div>
            <div>
              <p className="text-sm text-gray-400">Plan</p>
              <p className="text-2xl font-semibold text-white">
                {getPlanDisplayName(team.plan)}
              </p>
            </div>
            <div>
              <p className="text-sm text-gray-400">Created</p>
              <p className="text-2xl font-semibold text-white">
                {new Date(team.created_at).toLocaleDateString()}
              </p>
            </div>
          </div>
        </div>

        {/* Quick Actions */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
          <Link
            href={`/teams/${teamId}/members`}
            className="bg-gray-800 rounded-lg p-4 border border-gray-700 hover:border-gray-600 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="p-2 bg-blue-500/20 rounded-lg">
                <svg
                  className="w-6 h-6 text-blue-400"
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
              </div>
              <div>
                <p className="font-medium text-white">Members</p>
                <p className="text-sm text-gray-400">
                  Manage team members and roles
                </p>
              </div>
            </div>
          </Link>

          <Link
            href={`/teams/${teamId}/settings`}
            className="bg-gray-800 rounded-lg p-4 border border-gray-700 hover:border-gray-600 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="p-2 bg-purple-500/20 rounded-lg">
                <svg
                  className="w-6 h-6 text-purple-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                  />
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                  />
                </svg>
              </div>
              <div>
                <p className="font-medium text-white">Settings</p>
                <p className="text-sm text-gray-400">Team settings and plan</p>
              </div>
            </div>
          </Link>

          <Link
            href={`/teams/${teamId}/reports`}
            className="bg-gray-800 rounded-lg p-4 border border-gray-700 hover:border-gray-600 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="p-2 bg-green-500/20 rounded-lg">
                <svg
                  className="w-6 h-6 text-green-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
                  />
                </svg>
              </div>
              <div>
                <p className="font-medium text-white">Reports</p>
                <p className="text-sm text-gray-400">Usage and audit logs</p>
              </div>
            </div>
          </Link>
        </div>

        {/* Recent Members */}
        <div className="bg-gray-800 rounded-lg border border-gray-700">
          <div className="flex items-center justify-between p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Team Members</h2>
            <Link
              href={`/teams/${teamId}/members`}
              className="text-sm text-blue-400 hover:text-blue-300"
            >
              View all
            </Link>
          </div>
          <div className="divide-y divide-gray-700">
            {members.slice(0, 5).map((member) => (
              <div
                key={member.id}
                className="flex items-center justify-between p-4"
              >
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-gray-700 rounded-full flex items-center justify-center">
                    <span className="text-lg font-medium text-white">
                      {member.email?.[0].toUpperCase() || '?'}
                    </span>
                  </div>
                  <div>
                    <p className="font-medium text-white">{member.email}</p>
                    <p className="text-sm text-gray-400">
                      Joined {new Date(member.joined_at).toLocaleDateString()}
                    </p>
                  </div>
                </div>
                <span
                  className={`px-2 py-1 rounded text-xs font-medium ${
                    member.role === TeamRole.OWNER
                      ? 'bg-yellow-500/20 text-yellow-400'
                      : member.role === TeamRole.ADMIN
                        ? 'bg-purple-500/20 text-purple-400'
                        : member.role === TeamRole.MEMBER
                          ? 'bg-blue-500/20 text-blue-400'
                          : 'bg-gray-500/20 text-gray-400'
                  }`}
                >
                  {getRoleDisplayName(member.role)}
                </span>
              </div>
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
