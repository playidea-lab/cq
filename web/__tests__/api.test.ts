import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Import after mocking
import { sendMessage, getHistory, getStatus } from '@/lib/api';

describe('API Client', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  describe('sendMessage', () => {
    it('sends message and returns response', async () => {
      const mockResponse = {
        id: 'resp-123',
        conversation_id: 'conv-456',
        message: {
          id: 'msg-789',
          role: 'assistant',
          content: 'Hello!',
          timestamp: new Date().toISOString(),
        },
        done: true,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      });

      const result = await sendMessage('Hi there');

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:4000/api/chat/message',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
        })
      );

      expect(result.conversation_id).toBe('conv-456');
      expect(result.message.content).toBe('Hello!');
    });

    it('throws on API error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      });

      await expect(sendMessage('test')).rejects.toThrow('API error: 500');
    });
  });

  describe('getHistory', () => {
    it('fetches conversation history', async () => {
      const mockHistory = [
        { id: '1', role: 'user', content: 'Hi', timestamp: '2024-01-01' },
        { id: '2', role: 'assistant', content: 'Hello', timestamp: '2024-01-01' },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockHistory),
      });

      const result = await getHistory('conv-123');

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:4000/api/chat/history/conv-123'
      );
      expect(result).toHaveLength(2);
    });
  });

  describe('getStatus', () => {
    it('fetches C4 status', async () => {
      const mockStatus = {
        initialized: true,
        status: 'EXECUTE',
        queue: { pending: 10, done: 40 },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockStatus),
      });

      const result = await getStatus();

      expect(result.initialized).toBe(true);
      expect(result.status).toBe('EXECUTE');
    });
  });
});

describe('Message Interface', () => {
  it('has correct structure', () => {
    const message = {
      id: '123',
      role: 'user' as const,
      content: 'Test message',
      timestamp: new Date().toISOString(),
    };

    expect(message.role).toBe('user');
    expect(typeof message.content).toBe('string');
  });
});
