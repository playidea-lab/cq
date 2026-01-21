'use client';

import { useState, useEffect, useCallback } from 'react';

// =============================================================================
// Types
// =============================================================================

interface Conversation {
  id: string;
  title: string;
  lastMessage: string;
  updatedAt: Date;
}

interface ConversationSidebarProps {
  currentConversationId: string | null;
  onSelectConversation: (id: string) => void;
  onNewConversation: () => void;
}

// =============================================================================
// Local Storage Keys
// =============================================================================

const STORAGE_KEY = 'c4_conversations';

// =============================================================================
// Helper Functions
// =============================================================================

function loadConversations(): Conversation[] {
  if (typeof window === 'undefined') return [];

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return [];

    const parsed = JSON.parse(stored);
    return parsed.map((c: Conversation) => ({
      ...c,
      updatedAt: new Date(c.updatedAt),
    }));
  } catch {
    return [];
  }
}

function saveConversations(conversations: Conversation[]): void {
  if (typeof window === 'undefined') return;

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(conversations));
  } catch {
    console.error('Failed to save conversations to localStorage');
  }
}

function extractTitle(message: string): string {
  // Take first line or first 50 chars
  const firstLine = message.split('\n')[0].trim();
  if (firstLine.length <= 50) return firstLine;
  return firstLine.slice(0, 47) + '...';
}

function formatRelativeTime(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

// =============================================================================
// Conversation Item Component
// =============================================================================

function ConversationItem({
  conversation,
  isActive,
  onSelect,
  onDelete,
}: {
  conversation: Conversation;
  isActive: boolean;
  onSelect: () => void;
  onDelete: () => void;
}) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (showDeleteConfirm) {
      onDelete();
      setShowDeleteConfirm(false);
    } else {
      setShowDeleteConfirm(true);
      // Auto-hide after 3 seconds
      setTimeout(() => setShowDeleteConfirm(false), 3000);
    }
  };

  return (
    <div
      onClick={onSelect}
      className={`group relative px-3 py-2 rounded-lg cursor-pointer transition-colors ${
        isActive
          ? 'bg-blue-600/20 border border-blue-500/50'
          : 'hover:bg-gray-800 border border-transparent'
      }`}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <p className={`text-sm font-medium truncate ${
            isActive ? 'text-blue-400' : 'text-gray-200'
          }`}>
            {conversation.title || 'New Conversation'}
          </p>
          <p className="text-xs text-gray-500 truncate mt-0.5">
            {conversation.lastMessage || 'No messages'}
          </p>
        </div>
        <div className="flex-shrink-0 flex items-center gap-1">
          <span className="text-xs text-gray-600">
            {formatRelativeTime(conversation.updatedAt)}
          </span>
          <button
            onClick={handleDelete}
            className={`p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity ${
              showDeleteConfirm
                ? 'bg-red-600 text-white opacity-100'
                : 'hover:bg-gray-700 text-gray-400'
            }`}
            title={showDeleteConfirm ? 'Click again to delete' : 'Delete conversation'}
          >
            {showDeleteConfirm ? (
              <span className="text-xs px-1">Delete?</span>
            ) : (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}

// =============================================================================
// Main ConversationSidebar Component
// =============================================================================

export default function ConversationSidebar({
  currentConversationId,
  onSelectConversation,
  onNewConversation,
}: ConversationSidebarProps) {
  // Initialize from localStorage synchronously to avoid flash
  const [conversations, setConversations] = useState<Conversation[]>(() => loadConversations());
  const isLoaded = true;

  // Save conversations to localStorage when changed
  useEffect(() => {
    if (isLoaded) {
      saveConversations(conversations);
    }
  }, [conversations, isLoaded]);

  // Add or update conversation
  const upsertConversation = useCallback((id: string, firstMessage: string) => {
    setConversations((prev) => {
      const existing = prev.find((c) => c.id === id);
      if (existing) {
        // Update existing
        return prev.map((c) =>
          c.id === id
            ? {
                ...c,
                lastMessage: extractTitle(firstMessage),
                updatedAt: new Date(),
              }
            : c
        ).sort((a, b) => b.updatedAt.getTime() - a.updatedAt.getTime());
      } else {
        // Add new
        return [
          {
            id,
            title: extractTitle(firstMessage),
            lastMessage: extractTitle(firstMessage),
            updatedAt: new Date(),
          },
          ...prev,
        ];
      }
    });
  }, []);

  const deleteConversation = useCallback((id: string) => {
    setConversations((prev) => prev.filter((c) => c.id !== id));
    // If deleting current conversation, start new one
    if (currentConversationId === id) {
      onNewConversation();
    }
  }, [currentConversationId, onNewConversation]);

  // Expose upsertConversation for parent to call
  useEffect(() => {
    // Attach to window for parent access (simple approach)
    (window as unknown as { __c4_upsert_conversation?: typeof upsertConversation }).__c4_upsert_conversation = upsertConversation;
    return () => {
      delete (window as unknown as { __c4_upsert_conversation?: typeof upsertConversation }).__c4_upsert_conversation;
    };
  }, [upsertConversation]);

  return (
    <div className="flex flex-col h-full bg-gray-900 border-r border-gray-700">
      {/* Header */}
      <div className="p-4 border-b border-gray-700">
        <button
          onClick={onNewConversation}
          className="w-full flex items-center justify-center gap-2 bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New Chat
        </button>
      </div>

      {/* Conversations List */}
      <div className="flex-1 overflow-y-auto p-2 space-y-1">
        {conversations.length === 0 ? (
          <div className="text-center text-gray-500 mt-8 px-4">
            <p className="text-sm">No conversations yet</p>
            <p className="text-xs mt-1">Start a new chat to begin</p>
          </div>
        ) : (
          conversations.map((conv) => (
            <ConversationItem
              key={conv.id}
              conversation={conv}
              isActive={conv.id === currentConversationId}
              onSelect={() => onSelectConversation(conv.id)}
              onDelete={() => deleteConversation(conv.id)}
            />
          ))
        )}
      </div>

      {/* Footer */}
      <div className="p-3 border-t border-gray-700 text-center">
        <p className="text-xs text-gray-600">
          {conversations.length} conversation{conversations.length !== 1 ? 's' : ''}
        </p>
      </div>
    </div>
  );
}

// =============================================================================
// Exported Hook for Parent Component
// =============================================================================

export function useConversationManager() {
  const upsertConversation = useCallback((id: string, firstMessage: string) => {
    const fn = (window as unknown as { __c4_upsert_conversation?: (id: string, msg: string) => void }).__c4_upsert_conversation;
    if (fn) {
      fn(id, firstMessage);
    }
  }, []);

  return { upsertConversation };
}
