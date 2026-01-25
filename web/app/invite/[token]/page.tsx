'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  TeamInviteDetails,
  InviteStatus,
  getInviteByToken,
  acceptInvite,
  getRoleDisplayName,
} from '@/lib/teams';

// Mock data for development
const mockInvite: TeamInviteDetails = {
  id: 'invite-1',
  team_id: 'team-1',
  team_name: 'Acme Corp',
  email: 'invited@example.com',
  role: 'member' as any,
  status: InviteStatus.PENDING,
  expires_at: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(), // 7 days from now
  inviter_email: 'admin@acme.com',
};

type InviteState = 'loading' | 'valid' | 'expired' | 'accepted' | 'invalid' | 'error';

export default function InviteAcceptPage() {
  const params = useParams();
  const router = useRouter();
  const token = params.token as string;

  const [invite, setInvite] = useState<TeamInviteDetails | null>(null);
  const [state, setState] = useState<InviteState>('loading');
  const [accepting, setAccepting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchInvite() {
      try {
        // TODO: Replace with actual API call
        // const data = await getInviteByToken(token);

        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500));

        // Mock: check token patterns for demo
        if (token === 'expired') {
          setState('expired');
          return;
        }
        if (token === 'invalid') {
          setState('invalid');
          return;
        }
        if (token === 'accepted') {
          setState('accepted');
          setInvite(mockInvite);
          return;
        }

        setInvite(mockInvite);

        // Check if invite is expired
        if (mockInvite.expires_at && new Date(mockInvite.expires_at) < new Date()) {
          setState('expired');
        } else if (mockInvite.status === InviteStatus.ACCEPTED) {
          setState('accepted');
        } else if (mockInvite.status === InviteStatus.EXPIRED) {
          setState('expired');
        } else {
          setState('valid');
        }
      } catch (err) {
        console.error('Failed to fetch invite:', err);
        setState('error');
        setError(err instanceof Error ? err.message : 'Failed to load invitation');
      }
    }

    fetchInvite();
  }, [token]);

  const handleAccept = async () => {
    setAccepting(true);
    setError(null);

    try {
      // TODO: Replace with actual API call
      // await acceptInvite(token);

      // Simulate API call
      await new Promise((resolve) => setTimeout(resolve, 1000));

      // Redirect to team page
      router.push(`/teams/${invite?.team_id}`);
    } catch (err) {
      console.error('Failed to accept invite:', err);
      setError(err instanceof Error ? err.message : 'Failed to accept invitation');
      setAccepting(false);
    }
  };

  // Loading state
  if (state === 'loading') {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <main className="flex-1 flex items-center justify-center">
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-blue-500 mx-auto mb-4" />
            <p className="text-gray-400">Loading invitation...</p>
          </div>
        </main>
      </div>
    );
  }

  // Invalid token
  if (state === 'invalid') {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <main className="flex-1 flex items-center justify-center p-8">
          <div className="max-w-md w-full text-center">
            <div className="mb-6">
              <div className="w-16 h-16 bg-red-500/20 rounded-full flex items-center justify-center mx-auto">
                <svg
                  className="w-8 h-8 text-red-400"
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
              </div>
            </div>
            <h1 className="text-2xl font-bold text-white mb-2">
              Invalid Invitation
            </h1>
            <p className="text-gray-400 mb-6">
              This invitation link is invalid or has been revoked.
              Please contact the team administrator for a new invitation.
            </p>
            <Link
              href="/"
              className="inline-block bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700 transition-colors"
            >
              Go to Home
            </Link>
          </div>
        </main>
      </div>
    );
  }

  // Expired invitation
  if (state === 'expired') {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <main className="flex-1 flex items-center justify-center p-8">
          <div className="max-w-md w-full text-center">
            <div className="mb-6">
              <div className="w-16 h-16 bg-yellow-500/20 rounded-full flex items-center justify-center mx-auto">
                <svg
                  className="w-8 h-8 text-yellow-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
              </div>
            </div>
            <h1 className="text-2xl font-bold text-white mb-2">
              Invitation Expired
            </h1>
            <p className="text-gray-400 mb-6">
              This invitation has expired. Please contact the team administrator
              to send you a new invitation.
            </p>
            <Link
              href="/"
              className="inline-block bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700 transition-colors"
            >
              Go to Home
            </Link>
          </div>
        </main>
      </div>
    );
  }

  // Already accepted
  if (state === 'accepted') {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <main className="flex-1 flex items-center justify-center p-8">
          <div className="max-w-md w-full text-center">
            <div className="mb-6">
              <div className="w-16 h-16 bg-green-500/20 rounded-full flex items-center justify-center mx-auto">
                <svg
                  className="w-8 h-8 text-green-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              </div>
            </div>
            <h1 className="text-2xl font-bold text-white mb-2">
              Already Accepted
            </h1>
            <p className="text-gray-400 mb-6">
              You have already accepted this invitation.
              {invite && ` You are now a member of ${invite.team_name}.`}
            </p>
            {invite && (
              <Link
                href={`/teams/${invite.team_id}`}
                className="inline-block bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700 transition-colors"
              >
                Go to Team
              </Link>
            )}
          </div>
        </main>
      </div>
    );
  }

  // Error state
  if (state === 'error') {
    return (
      <div className="flex flex-col min-h-screen bg-gray-900">
        <main className="flex-1 flex items-center justify-center p-8">
          <div className="max-w-md w-full text-center">
            <div className="mb-6">
              <div className="w-16 h-16 bg-red-500/20 rounded-full flex items-center justify-center mx-auto">
                <svg
                  className="w-8 h-8 text-red-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                  />
                </svg>
              </div>
            </div>
            <h1 className="text-2xl font-bold text-white mb-2">
              Something Went Wrong
            </h1>
            <p className="text-gray-400 mb-6">
              {error || 'Failed to load invitation. Please try again.'}
            </p>
            <button
              onClick={() => window.location.reload()}
              className="inline-block bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700 transition-colors"
            >
              Try Again
            </button>
          </div>
        </main>
      </div>
    );
  }

  // Valid invitation - show accept form
  return (
    <div className="flex flex-col min-h-screen bg-gray-900">
      <main className="flex-1 flex items-center justify-center p-8">
        <div className="max-w-md w-full">
          {/* Logo */}
          <div className="text-center mb-8">
            <Link href="/" className="inline-flex items-center gap-2">
              <span className="text-2xl font-bold text-white">C4</span>
              <span className="text-sm text-gray-400">Cloud</span>
            </Link>
          </div>

          {/* Invitation Card */}
          <div className="bg-gray-800 rounded-lg border border-gray-700 p-6">
            <div className="text-center mb-6">
              <div className="w-16 h-16 bg-blue-500/20 rounded-full flex items-center justify-center mx-auto mb-4">
                <svg
                  className="w-8 h-8 text-blue-400"
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
              </div>
              <h1 className="text-xl font-bold text-white mb-2">
                You&apos;re Invited!
              </h1>
              <p className="text-gray-400">
                You&apos;ve been invited to join a team on C4 Cloud.
              </p>
            </div>

            {/* Invitation Details */}
            <div className="bg-gray-700/50 rounded-lg p-4 mb-6 space-y-3">
              <div className="flex justify-between">
                <span className="text-gray-400">Team</span>
                <span className="text-white font-medium">{invite?.team_name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Your Role</span>
                <span className="text-white font-medium">
                  {invite?.role && getRoleDisplayName(invite.role)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Invited By</span>
                <span className="text-white font-medium">
                  {invite?.inviter_email || 'Team Administrator'}
                </span>
              </div>
              {invite?.expires_at && (
                <div className="flex justify-between">
                  <span className="text-gray-400">Expires</span>
                  <span className="text-white font-medium">
                    {new Date(invite.expires_at).toLocaleDateString()}
                  </span>
                </div>
              )}
            </div>

            {/* Error Message */}
            {error && (
              <div className="bg-red-500/20 border border-red-500/50 rounded-lg p-3 mb-4">
                <p className="text-red-400 text-sm">{error}</p>
              </div>
            )}

            {/* Action Buttons */}
            <div className="space-y-3">
              <button
                onClick={handleAccept}
                disabled={accepting}
                className="w-full bg-blue-600 text-white py-3 rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
              >
                {accepting ? (
                  <span className="flex items-center justify-center gap-2">
                    <svg
                      className="animate-spin h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        className="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        strokeWidth="4"
                      />
                      <path
                        className="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                      />
                    </svg>
                    Accepting...
                  </span>
                ) : (
                  'Accept Invitation'
                )}
              </button>
              <Link
                href="/"
                className="block text-center text-gray-400 hover:text-white transition-colors py-2"
              >
                Decline
              </Link>
            </div>
          </div>

          {/* Footer Note */}
          <p className="text-center text-gray-500 text-sm mt-6">
            By accepting this invitation, you agree to the team&apos;s policies
            and will be able to access shared resources.
          </p>
        </div>
      </main>
    </div>
  );
}
