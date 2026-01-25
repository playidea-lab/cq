'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import Header from '../../../components/Header';
import {
  UsageReport,
  ActivityLog,
  AuditLog,
  getUsageReport,
  getTeamActivities,
  getAuditLogs,
  downloadAuditLogs,
  formatDuration,
  getActivityTypeDisplayName,
  getActionDisplayName,
  getActorTypeDisplayName,
  ExportFormat,
} from '@/lib/reports';

type TabType = 'overview' | 'activities' | 'audit';

export default function ReportsPage() {
  const params = useParams();
  const teamId = params.teamId as string;

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [usageReport, setUsageReport] = useState<UsageReport | null>(null);
  const [activities, setActivities] = useState<ActivityLog[]>([]);
  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([]);
  const [exporting, setExporting] = useState(false);

  useEffect(() => {
    async function fetchData() {
      try {
        const today = new Date();
        const thirtyDaysAgo = new Date(today.getTime() - 30 * 24 * 60 * 60 * 1000);
        const startDate = thirtyDaysAgo.toISOString().split('T')[0];
        const endDate = today.toISOString().split('T')[0];

        const [usage, acts, auditResponse] = await Promise.all([
          getUsageReport(teamId, startDate, endDate),
          getTeamActivities(teamId, { limit: 50 }),
          getAuditLogs(teamId, { limit: 50 }),
        ]);

        setUsageReport(usage);
        setActivities(acts);
        setAuditLogs(auditResponse.logs);
      } catch (err) {
        console.error('Failed to load reports:', err);
        setError(err instanceof Error ? err.message : 'Failed to load reports');
      } finally {
        setLoading(false);
      }
    }

    fetchData();
  }, [teamId]);

  const handleExport = async (format: ExportFormat) => {
    setExporting(true);
    try {
      await downloadAuditLogs(teamId, format);
    } catch (err) {
      console.error('Failed to export:', err);
      alert(err instanceof Error ? err.message : 'Failed to export');
    } finally {
      setExporting(false);
    }
  };

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

  if (error) {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <Header />
        <main className="flex-1 flex items-center justify-center">
          <div className="text-center">
            <p className="text-red-400 mb-4">{error}</p>
            <Link href={`/teams/${teamId}`} className="text-blue-400 hover:text-blue-300">
              Back to Team
            </Link>
          </div>
        </main>
      </div>
    );
  }

  // Calculate max for chart
  const maxCount = usageReport
    ? Math.max(...usageReport.by_date.map((d) => d.activity_count))
    : 100;

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
          <Link href={`/teams/${teamId}`} className="hover:text-white">
            Team
          </Link>
          <span>/</span>
          <span className="text-white">Reports</span>
        </div>

        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-2xl font-bold text-white">Reports & Analytics</h1>
            <p className="text-gray-400 text-sm mt-1">
              Usage metrics, activity logs, and audit trail
            </p>
          </div>
          <div className="flex gap-2">
            <button
              onClick={() => handleExport('csv')}
              disabled={exporting}
              className="px-4 py-2 border border-gray-600 text-gray-300 rounded-lg hover:bg-gray-700 transition-colors disabled:opacity-50 flex items-center gap-2"
            >
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
                  d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
                />
              </svg>
              Export CSV
            </button>
            <button
              onClick={() => handleExport('json')}
              disabled={exporting}
              className="px-4 py-2 border border-gray-600 text-gray-300 rounded-lg hover:bg-gray-700 transition-colors disabled:opacity-50 flex items-center gap-2"
            >
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
                  d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
                />
              </svg>
              Export JSON
            </button>
          </div>
        </div>

        {/* Tabs */}
        <div className="flex gap-1 mb-6 border-b border-gray-700">
          {(['overview', 'activities', 'audit'] as TabType[]).map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
                activeTab === tab
                  ? 'text-blue-400 border-blue-400'
                  : 'text-gray-400 border-transparent hover:text-white'
              }`}
            >
              {tab === 'overview'
                ? 'Overview'
                : tab === 'activities'
                  ? 'Activity Log'
                  : 'Audit Log'}
            </button>
          ))}
        </div>

        {/* Overview Tab */}
        {activeTab === 'overview' && usageReport && (
          <div className="space-y-6">
            {/* Stats Grid */}
            <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
              <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
                <p className="text-sm text-gray-400">Total Activities</p>
                <p className="text-2xl font-bold text-white mt-1">
                  {usageReport.total_activities.toLocaleString()}
                </p>
              </div>
              <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
                <p className="text-sm text-gray-400">Total Duration</p>
                <p className="text-2xl font-bold text-white mt-1">
                  {formatDuration(usageReport.total_duration_seconds)}
                </p>
              </div>
              <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
                <p className="text-sm text-gray-400">Tasks Completed</p>
                <p className="text-2xl font-bold text-green-400 mt-1">
                  {usageReport.by_type.task_completed?.toLocaleString() || 0}
                </p>
              </div>
              <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
                <p className="text-sm text-gray-400">PRs Created</p>
                <p className="text-2xl font-bold text-purple-400 mt-1">
                  {usageReport.by_type.pr_created?.toLocaleString() || 0}
                </p>
              </div>
            </div>

            {/* Activity Chart */}
            <div className="bg-gray-800 rounded-lg border border-gray-700 p-6">
              <h2 className="font-semibold text-white mb-4">Daily Activity (Last 7 Days)</h2>
              <div className="flex items-end gap-2 h-48">
                {usageReport.by_date.map((day, index) => {
                  const height = (day.activity_count / maxCount) * 100;
                  const date = new Date(day.date);
                  const dayName = date.toLocaleDateString('en-US', { weekday: 'short' });
                  return (
                    <div key={index} className="flex-1 flex flex-col items-center gap-2">
                      <div className="w-full bg-gray-700 rounded-t relative" style={{ height: '180px' }}>
                        <div
                          className="absolute bottom-0 w-full bg-blue-500 rounded-t transition-all"
                          style={{ height: `${height}%` }}
                        />
                      </div>
                      <div className="text-center">
                        <p className="text-xs text-gray-400">{dayName}</p>
                        <p className="text-sm font-medium text-white">{day.activity_count}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Activity Breakdown */}
            <div className="bg-gray-800 rounded-lg border border-gray-700 p-6">
              <h2 className="font-semibold text-white mb-4">Activity Breakdown</h2>
              <div className="space-y-3">
                {Object.entries(usageReport.by_type).map(([type, count]) => {
                  const percentage = (count / usageReport.total_activities) * 100;
                  return (
                    <div key={type}>
                      <div className="flex items-center justify-between mb-1">
                        <span className="text-sm text-gray-300">
                          {getActivityTypeDisplayName(type)}
                        </span>
                        <span className="text-sm text-gray-400">
                          {count.toLocaleString()} ({percentage.toFixed(1)}%)
                        </span>
                      </div>
                      <div className="w-full bg-gray-700 rounded-full h-2">
                        <div
                          className="bg-blue-500 rounded-full h-2 transition-all"
                          style={{ width: `${percentage}%` }}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}

        {/* Activities Tab */}
        {activeTab === 'activities' && (
          <div className="bg-gray-800 rounded-lg border border-gray-700">
            <div className="p-4 border-b border-gray-700">
              <h2 className="font-semibold text-white">Recent Activities</h2>
            </div>
            <div className="divide-y divide-gray-700">
              {activities.length === 0 ? (
                <div className="p-8 text-center text-gray-400">
                  No activities recorded yet.
                </div>
              ) : (
                activities.map((activity) => (
                  <div key={activity.id} className="p-4 flex items-start gap-4">
                    <div className="mt-1">
                      <ActivityIcon type={activity.activity_type} />
                    </div>
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-white">
                          {getActivityTypeDisplayName(activity.activity_type)}
                        </span>
                        <span className="text-xs px-2 py-0.5 bg-gray-700 text-gray-300 rounded">
                          {activity.resource_type}/{activity.resource_id}
                        </span>
                      </div>
                      <p className="text-sm text-gray-400 mt-1">
                        {(activity.metadata?.task_title as string) ||
                          (activity.metadata?.pr_title as string) ||
                          (activity.metadata?.checkpoint_name as string) ||
                          'No description'}
                      </p>
                      <div className="flex items-center gap-4 mt-2 text-xs text-gray-500">
                        <span>
                          {activity.created_at
                            ? new Date(activity.created_at).toLocaleString()
                            : 'Unknown time'}
                        </span>
                        {activity.duration_seconds && (
                          <span>Duration: {formatDuration(activity.duration_seconds)}</span>
                        )}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        )}

        {/* Audit Tab */}
        {activeTab === 'audit' && (
          <div className="bg-gray-800 rounded-lg border border-gray-700">
            <div className="p-4 border-b border-gray-700 flex items-center justify-between">
              <h2 className="font-semibold text-white">Audit Log</h2>
              <span className="text-sm text-gray-400">
                {auditLogs.length} entries
              </span>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="text-left text-sm text-gray-400 border-b border-gray-700">
                    <th className="p-4 font-medium">Time</th>
                    <th className="p-4 font-medium">Actor</th>
                    <th className="p-4 font-medium">Action</th>
                    <th className="p-4 font-medium">Resource</th>
                    <th className="p-4 font-medium">Details</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-700">
                  {auditLogs.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="p-8 text-center text-gray-400">
                        No audit logs recorded yet.
                      </td>
                    </tr>
                  ) : (
                    auditLogs.map((log) => (
                      <tr key={log.id} className="hover:bg-gray-700/50">
                        <td className="p-4 text-sm text-gray-300">
                          {log.created_at
                            ? new Date(log.created_at).toLocaleString()
                            : 'Unknown'}
                        </td>
                        <td className="p-4">
                          <div className="flex items-center gap-2">
                            <span
                              className={`text-xs px-2 py-0.5 rounded ${
                                log.actor_type === 'user'
                                  ? 'bg-blue-500/20 text-blue-400'
                                  : log.actor_type === 'system'
                                    ? 'bg-purple-500/20 text-purple-400'
                                    : 'bg-gray-500/20 text-gray-400'
                              }`}
                            >
                              {getActorTypeDisplayName(log.actor_type)}
                            </span>
                            <span className="text-sm text-white">
                              {log.actor_email || log.actor_id}
                            </span>
                          </div>
                        </td>
                        <td className="p-4 text-sm text-gray-300">
                          {getActionDisplayName(log.action)}
                        </td>
                        <td className="p-4 text-sm text-gray-400">
                          {log.resource_type}/{log.resource_id}
                        </td>
                        <td className="p-4">
                          {log.new_value && (
                            <button
                              onClick={() => {
                                console.log('Changes:', {
                                  old: log.old_value,
                                  new: log.new_value,
                                });
                              }}
                              className="text-xs text-blue-400 hover:underline"
                            >
                              View changes
                            </button>
                          )}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

// Activity icon component
function ActivityIcon({ type }: { type: string }) {
  const iconMap: Record<string, { bg: string; icon: React.ReactNode }> = {
    task_started: {
      bg: 'bg-blue-500/20',
      icon: (
        <svg className="w-5 h-5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      ),
    },
    task_completed: {
      bg: 'bg-green-500/20',
      icon: (
        <svg className="w-5 h-5 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      ),
    },
    pr_created: {
      bg: 'bg-purple-500/20',
      icon: (
        <svg className="w-5 h-5 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
        </svg>
      ),
    },
    review_submitted: {
      bg: 'bg-yellow-500/20',
      icon: (
        <svg className="w-5 h-5 text-yellow-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
        </svg>
      ),
    },
    checkpoint_approved: {
      bg: 'bg-green-500/20',
      icon: (
        <svg className="w-5 h-5 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4M7.835 4.697a3.42 3.42 0 001.946-.806 3.42 3.42 0 014.438 0 3.42 3.42 0 001.946.806 3.42 3.42 0 013.138 3.138 3.42 3.42 0 00.806 1.946 3.42 3.42 0 010 4.438 3.42 3.42 0 00-.806 1.946 3.42 3.42 0 01-3.138 3.138 3.42 3.42 0 00-1.946.806 3.42 3.42 0 01-4.438 0 3.42 3.42 0 00-1.946-.806 3.42 3.42 0 01-3.138-3.138 3.42 3.42 0 00-.806-1.946 3.42 3.42 0 010-4.438 3.42 3.42 0 00.806-1.946 3.42 3.42 0 013.138-3.138z" />
        </svg>
      ),
    },
  };

  const { bg, icon } = iconMap[type] || {
    bg: 'bg-gray-500/20',
    icon: (
      <svg className="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
    ),
  };

  return (
    <div className={`p-2 rounded-full ${bg}`}>
      {icon}
    </div>
  );
}
