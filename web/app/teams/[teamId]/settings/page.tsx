'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import Header from '../../../components/Header';
import {
  Team,
  TeamPlan,
  getTeam,
  updateTeam,
  deleteTeam,
  getPlanDisplayName,
} from '@/lib/teams';

export default function SettingsPage() {
  const params = useParams();
  const router = useRouter();
  const teamId = params.teamId as string;

  const [team, setTeam] = useState<Team | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Form state
  const [name, setName] = useState('');
  const [notifications, setNotifications] = useState(true);
  const [aiReview, setAiReview] = useState(true);
  const [autoMerge, setAutoMerge] = useState(false);

  useEffect(() => {
    async function fetchData() {
      try {
        const teamData = await getTeam(teamId);
        setTeam(teamData);
        setName(teamData.name);
        setNotifications((teamData.settings?.notifications as boolean) ?? true);
        setAiReview((teamData.settings?.ai_review as boolean) ?? true);
        setAutoMerge((teamData.settings?.auto_merge as boolean) ?? false);
      } catch (err) {
        console.error('Failed to load team:', err);
        setError(err instanceof Error ? err.message : 'Failed to load team');
      } finally {
        setLoading(false);
      }
    }

    fetchData();
  }, [teamId]);

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      const updatedTeam = await updateTeam(teamId, {
        name,
        settings: { notifications, ai_review: aiReview, auto_merge: autoMerge },
      });
      setTeam(updatedTeam);
    } catch (err) {
      console.error('Failed to save settings:', err);
      setError(err instanceof Error ? err.message : 'Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (
      !confirm(
        'Are you sure you want to delete this team? This action cannot be undone.'
      )
    )
      return;

    setDeleting(true);
    try {
      await deleteTeam(teamId);
      router.push('/teams');
    } catch (err) {
      console.error('Failed to delete team:', err);
      setError(err instanceof Error ? err.message : 'Failed to delete team');
      setDeleting(false);
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
      <main className="flex-1 max-w-4xl w-full mx-auto p-8">
        {/* Breadcrumb */}
        <div className="flex items-center gap-2 text-sm text-gray-400 mb-6">
          <Link href="/teams" className="hover:text-white">
            Teams
          </Link>
          <span>/</span>
          <Link href={`/teams/${teamId}`} className="hover:text-white">
            {team?.name}
          </Link>
          <span>/</span>
          <span className="text-white">Settings</span>
        </div>

        <h1 className="text-2xl font-bold text-white mb-8">Team Settings</h1>

        {/* General Settings */}
        <div className="bg-gray-800 rounded-lg border border-gray-700 mb-6">
          <div className="p-4 border-b border-gray-700">
            <h2 className="font-semibold text-white">General</h2>
          </div>
          <div className="p-6 space-y-6">
            <div>
              <label
                htmlFor="name"
                className="block text-sm font-medium text-gray-300 mb-2"
              >
                Team Name
              </label>
              <input
                type="text"
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full max-w-md bg-gray-700 border border-gray-600 rounded-lg px-4 py-2 text-white focus:outline-none focus:border-blue-500"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-300 mb-2">
                URL Slug
              </label>
              <p className="text-gray-400">/{team?.slug}</p>
              <p className="text-xs text-gray-500 mt-1">
                Contact support to change the URL slug
              </p>
            </div>
          </div>
        </div>

        {/* Plan Information */}
        <div className="bg-gray-800 rounded-lg border border-gray-700 mb-6">
          <div className="p-4 border-b border-gray-700">
            <h2 className="font-semibold text-white">Plan & Billing</h2>
          </div>
          <div className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <div className="flex items-center gap-3 mb-2">
                  <span
                    className={`px-3 py-1 rounded text-sm font-medium text-white ${
                      planColors[team?.plan || TeamPlan.FREE]
                    }`}
                  >
                    {getPlanDisplayName(team?.plan || TeamPlan.FREE)}
                  </span>
                </div>
                <p className="text-sm text-gray-400">
                  Manage your subscription and billing details
                </p>
              </div>
              <button className="px-4 py-2 border border-gray-600 text-gray-300 rounded-lg hover:bg-gray-700 transition-colors">
                Manage Billing
              </button>
            </div>
          </div>
        </div>

        {/* Feature Settings */}
        <div className="bg-gray-800 rounded-lg border border-gray-700 mb-6">
          <div className="p-4 border-b border-gray-700">
            <h2 className="font-semibold text-white">Features</h2>
          </div>
          <div className="p-6 space-y-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium text-white">Email Notifications</p>
                <p className="text-sm text-gray-400">
                  Receive email updates about team activity
                </p>
              </div>
              <button
                onClick={() => setNotifications(!notifications)}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  notifications ? 'bg-blue-600' : 'bg-gray-600'
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    notifications ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>

            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium text-white">AI Code Review</p>
                <p className="text-sm text-gray-400">
                  Automatically review pull requests with AI
                </p>
              </div>
              <button
                onClick={() => setAiReview(!aiReview)}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  aiReview ? 'bg-blue-600' : 'bg-gray-600'
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    aiReview ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>

            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium text-white">Auto Merge</p>
                <p className="text-sm text-gray-400">
                  Automatically merge approved pull requests
                </p>
              </div>
              <button
                onClick={() => setAutoMerge(!autoMerge)}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  autoMerge ? 'bg-blue-600' : 'bg-gray-600'
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    autoMerge ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>
          </div>
        </div>

        {/* Save Button */}
        <div className="flex justify-end mb-8">
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save Changes'}
          </button>
        </div>

        {/* Danger Zone */}
        <div className="bg-gray-800 rounded-lg border border-red-500/50">
          <div className="p-4 border-b border-red-500/50">
            <h2 className="font-semibold text-red-400">Danger Zone</h2>
          </div>
          <div className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium text-white">Delete Team</p>
                <p className="text-sm text-gray-400">
                  Permanently delete this team and all its data
                </p>
              </div>
              <button
                onClick={handleDelete}
                disabled={deleting}
                className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors disabled:opacity-50"
              >
                {deleting ? 'Deleting...' : 'Delete Team'}
              </button>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
