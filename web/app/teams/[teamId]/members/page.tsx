'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import Header from '../../../components/Header';
import {
  Team,
  TeamMember,
  TeamInvite,
  TeamRole,
  InviteStatus,
  getTeam,
  listMembers,
  listInvites,
  inviteMember,
  updateMemberRole,
  removeMember,
  cancelInvite,
  getRoleDisplayName,
} from '@/lib/teams';

export default function MembersPage() {
  const params = useParams();
  const teamId = params.teamId as string;

  const [team, setTeam] = useState<Team | null>(null);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [invites, setInvites] = useState<TeamInvite[]>([]);
  const [loading, setLoading] = useState(true);
  const [showInviteModal, setShowInviteModal] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchData() {
      try {
        const [teamData, membersData, invitesData] = await Promise.all([
          getTeam(teamId),
          listMembers(teamId),
          listInvites(teamId),
        ]);
        setTeam(teamData);
        setMembers(membersData);
        setInvites(invitesData);
      } catch (err) {
        console.error('Failed to load members:', err);
        setError(err instanceof Error ? err.message : 'Failed to load members');
      } finally {
        setLoading(false);
      }
    }

    fetchData();
  }, [teamId]);

  const handleRoleChange = async (memberId: string, newRole: TeamRole) => {
    try {
      await updateMemberRole(teamId, memberId, { role: newRole });
      setMembers(
        members.map((m) => (m.id === memberId ? { ...m, role: newRole } : m))
      );
    } catch (err) {
      console.error('Failed to update role:', err);
      alert(err instanceof Error ? err.message : 'Failed to update role');
    }
  };

  const handleRemoveMember = async (memberId: string) => {
    if (!confirm('Are you sure you want to remove this member?')) return;

    try {
      await removeMember(teamId, memberId);
      setMembers(members.filter((m) => m.id !== memberId));
    } catch (err) {
      console.error('Failed to remove member:', err);
      alert(err instanceof Error ? err.message : 'Failed to remove member');
    }
  };

  const handleCancelInvite = async (inviteId: string) => {
    try {
      await cancelInvite(teamId, inviteId);
      setInvites(invites.filter((i) => i.id !== inviteId));
    } catch (err) {
      console.error('Failed to cancel invite:', err);
      alert(err instanceof Error ? err.message : 'Failed to cancel invite');
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
            {team?.name}
          </Link>
          <span>/</span>
          <span className="text-white">Members</span>
        </div>

        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-2xl font-bold text-white">Team Members</h1>
            <p className="text-gray-400 text-sm mt-1">
              Manage members and their roles
            </p>
          </div>
          <button
            onClick={() => setShowInviteModal(true)}
            className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700 transition-colors flex items-center gap-2"
          >
            <svg
              className="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z"
              />
            </svg>
            Invite Member
          </button>
        </div>

        {/* Members Table */}
        <div className="bg-gray-800 rounded-lg border border-gray-700 mb-8">
          <div className="p-4 border-b border-gray-700">
            <h2 className="font-semibold text-white">
              Members ({members.length})
            </h2>
          </div>
          <div className="divide-y divide-gray-700">
            {members.map((member) => (
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

                <div className="flex items-center gap-3">
                  {member.role === TeamRole.OWNER ? (
                    <span className="px-3 py-1 bg-yellow-500/20 text-yellow-400 rounded-lg text-sm font-medium">
                      Owner
                    </span>
                  ) : (
                    <select
                      value={member.role}
                      onChange={(e) =>
                        handleRoleChange(member.id, e.target.value as TeamRole)
                      }
                      className="bg-gray-700 border border-gray-600 rounded-lg px-3 py-1 text-sm text-white focus:outline-none focus:border-blue-500"
                    >
                      <option value={TeamRole.ADMIN}>Admin</option>
                      <option value={TeamRole.MEMBER}>Member</option>
                      <option value={TeamRole.VIEWER}>Viewer</option>
                    </select>
                  )}

                  {member.role !== TeamRole.OWNER && (
                    <button
                      onClick={() => handleRemoveMember(member.id)}
                      className="p-2 text-gray-400 hover:text-red-400 transition-colors"
                    >
                      <svg
                        className="w-5 h-5"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Pending Invites */}
        {invites.length > 0 && (
          <div className="bg-gray-800 rounded-lg border border-gray-700">
            <div className="p-4 border-b border-gray-700">
              <h2 className="font-semibold text-white">
                Pending Invites ({invites.length})
              </h2>
            </div>
            <div className="divide-y divide-gray-700">
              {invites.map((invite) => (
                <div
                  key={invite.id}
                  className="flex items-center justify-between p-4"
                >
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 bg-gray-700 rounded-full flex items-center justify-center">
                      <svg
                        className="w-5 h-5 text-gray-400"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
                        />
                      </svg>
                    </div>
                    <div>
                      <p className="font-medium text-white">{invite.email}</p>
                      <p className="text-sm text-gray-400">
                        Invited {new Date(invite.invited_at).toLocaleDateString()}{' '}
                        · {getRoleDisplayName(invite.role)}
                      </p>
                    </div>
                  </div>

                  <button
                    onClick={() => handleCancelInvite(invite.id)}
                    className="px-3 py-1 border border-gray-600 text-gray-300 rounded-lg hover:bg-gray-700 text-sm"
                  >
                    Cancel
                  </button>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Invite Modal */}
        {showInviteModal && (
          <InviteModal
            teamId={teamId}
            onClose={() => setShowInviteModal(false)}
            onInvite={(invite) => {
              setInvites([...invites, invite]);
              setShowInviteModal(false);
            }}
          />
        )}
      </main>
    </div>
  );
}

interface InviteModalProps {
  teamId: string;
  onClose: () => void;
  onInvite: (invite: TeamInvite) => void;
}

function InviteModal({ teamId, onClose, onInvite }: InviteModalProps) {
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<TeamRole>(TeamRole.MEMBER);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const invite = await inviteMember(teamId, { email, role });
      onInvite(invite);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to send invite');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-gray-800 rounded-lg p-6 w-full max-w-md border border-gray-700">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-xl font-semibold text-white">Invite Member</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-white">
            <svg
              className="w-6 h-6"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="space-y-4">
            <div>
              <label
                htmlFor="email"
                className="block text-sm font-medium text-gray-300 mb-1"
              >
                Email Address
              </label>
              <input
                type="email"
                id="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="colleague@company.com"
                className="w-full bg-gray-700 border border-gray-600 rounded-lg px-4 py-2 text-white placeholder-gray-400 focus:outline-none focus:border-blue-500"
                required
              />
            </div>

            <div>
              <label
                htmlFor="role"
                className="block text-sm font-medium text-gray-300 mb-1"
              >
                Role
              </label>
              <select
                id="role"
                value={role}
                onChange={(e) => setRole(e.target.value as TeamRole)}
                className="w-full bg-gray-700 border border-gray-600 rounded-lg px-4 py-2 text-white focus:outline-none focus:border-blue-500"
              >
                <option value={TeamRole.ADMIN}>Admin - Full access</option>
                <option value={TeamRole.MEMBER}>
                  Member - Can create and manage
                </option>
                <option value={TeamRole.VIEWER}>Viewer - Read only</option>
              </select>
            </div>

            {error && <p className="text-red-400 text-sm">{error}</p>}
          </div>

          <div className="flex gap-3 mt-6">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 px-4 py-2 border border-gray-600 text-gray-300 rounded-lg hover:bg-gray-700 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={loading || !email}
              className="flex-1 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? 'Sending...' : 'Send Invite'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
