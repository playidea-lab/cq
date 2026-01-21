import { test, expect, Page } from '@playwright/test';

/**
 * C4 Chat UI E2E Tests
 *
 * Tests cover:
 * - Basic chat interactions
 * - SSE streaming visualization
 * - Tool call display
 * - Conversation sidebar
 * - Error handling
 * - Responsive design
 */

// =============================================================================
// Test Utilities
// =============================================================================

/**
 * Helper to wait for chat input to be ready
 */
async function waitForChatReady(page: Page) {
  await page.waitForSelector('input[placeholder*="message"]', {
    state: 'visible',
    timeout: 10000,
  });
}

/**
 * Helper to send a chat message
 */
async function sendMessage(page: Page, message: string) {
  const input = page.locator('input[placeholder*="message"]');
  await input.fill(message);
  await page.click('button:has-text("Send")');
}

/**
 * Helper to mock SSE response for testing
 */
async function mockSSEResponse(page: Page, events: Array<{ event: string; data: object }>) {
  await page.route('**/api/chat/message', async (route) => {
    const body = events
      .map((e) => `event: ${e.event}\ndata: ${JSON.stringify(e.data)}\n\n`)
      .join('');

    await route.fulfill({
      status: 200,
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        'Connection': 'keep-alive',
      },
      body,
    });
  });
}

// =============================================================================
// Basic Chat Tests
// =============================================================================

test.describe('Chat UI', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should display welcome message on initial load', async ({ page }) => {
    await expect(page.locator('text=Welcome to C4 Chat')).toBeVisible();
  });

  test('should have input field and send button', async ({ page }) => {
    const input = page.locator('input[placeholder*="message"]');
    const sendButton = page.locator('button:has-text("Send")');

    await expect(input).toBeVisible();
    await expect(sendButton).toBeVisible();
  });

  test('should disable send button when input is empty', async ({ page }) => {
    const sendButton = page.locator('button:has-text("Send")');
    await expect(sendButton).toBeDisabled();
  });

  test('should enable send button when input has text', async ({ page }) => {
    const input = page.locator('input[placeholder*="message"]');
    await input.fill('Hello');

    const sendButton = page.locator('button:has-text("Send")');
    await expect(sendButton).toBeEnabled();
  });

  test('should clear input after clicking send', async ({ page }) => {
    // Just test that input clears - SSE response testing is separate
    await page.route('**/api/chat/message', async (route) => {
      // Don't wait, just fulfill with minimal response
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
        body: `event: start\ndata: {"conversation_id": "test"}\n\n`,
      });
    });

    const input = page.locator('input[placeholder*="message"]');
    await input.fill('Test message');
    await page.click('button:has-text("Send")');

    // Input should be cleared after sending
    await expect(input).toHaveValue('');
  });
});

// =============================================================================
// SSE Streaming Tests
// =============================================================================

// SSE Mock은 Playwright에서 완벽히 시뮬레이션되지 않음
// 실제 백엔드 연동 시 테스트 필요
test.describe.skip('SSE Streaming', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should show loading state after sending message', async ({ page }) => {
    // Mock a slow response
    await page.route('**/api/chat/message', async (route) => {
      // Delay response
      await new Promise(resolve => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
        body: `event: start\ndata: {"conversation_id": "test"}\n\nevent: done\ndata: {"done": true}\n\n`,
      });
    });

    await sendMessage(page, 'Test');

    // Cancel button should appear during loading
    await expect(page.locator('button:has-text("Cancel")')).toBeVisible({ timeout: 2000 });
  });

  test('should display assistant response after streaming', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'chunk', data: { content: 'Response text here' } },
      { event: 'done', data: { conversation_id: 'test-conv', message: { id: '1', role: 'assistant', content: 'Response text here', timestamp: new Date().toISOString() }, success: true, turns: 1, total_tool_calls: 0, done: true } },
    ]);

    await sendMessage(page, 'Hello');

    // Final content should be visible
    await expect(page.locator('text=Response text here')).toBeVisible({ timeout: 5000 });
  });
});

// =============================================================================
// Tool Call Display Tests
// =============================================================================

// SSE Mock 필요 - 실제 백엔드 연동 시 테스트
test.describe.skip('Tool Call Display', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should display tool name when tool is used', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'chunk', data: { content: 'Reading file...' } },
      { event: 'tool_call', data: { name: 'read_file', input: { path: '/test.txt' } } },
      { event: 'tool_result', data: { name: 'read_file', result: 'file content', success: true, duration_ms: 100 } },
      { event: 'done', data: {
        conversation_id: 'test-conv',
        message: {
          id: '1',
          role: 'assistant',
          content: 'Reading file...',
          timestamp: new Date().toISOString(),
          tool_calls: [{ name: 'read_file', input: { path: '/test.txt' }, result: 'file content', success: true, duration_ms: 100 }]
        },
        success: true,
        turns: 1,
        total_tool_calls: 1,
        done: true
      }},
    ]);

    await sendMessage(page, 'Read file');

    // Tool name should be visible
    await expect(page.locator('text=read_file')).toBeVisible({ timeout: 10000 });
  });
});

// =============================================================================
// Conversation Sidebar Tests
// =============================================================================

test.describe('Conversation Sidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should display new chat button', async ({ page }) => {
    await expect(page.locator('button:has-text("New Chat")')).toBeVisible();
  });

  test('should show no conversations message initially', async ({ page }) => {
    await expect(page.locator('text=No conversations yet')).toBeVisible();
  });

  test('should show 0 conversations initially', async ({ page }) => {
    // Footer should show "0 conversations"
    await expect(page.locator('text=0 conversations')).toBeVisible();
  });

  test('should show welcome message after clicking New Chat', async ({ page }) => {
    // Click New Chat (even without prior conversation)
    await page.click('button:has-text("New Chat")');

    // Welcome message should still be visible
    await expect(page.locator('text=Welcome to C4 Chat')).toBeVisible({ timeout: 5000 });
  });
});

// =============================================================================
// Error Handling Tests
// =============================================================================

test.describe('Error Handling', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should display error message on API error', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'error', data: { error: 'Something went wrong' } },
    ]);

    await sendMessage(page, 'Trigger error');

    // Error message should be visible
    await expect(page.locator('text=Error:')).toBeVisible({ timeout: 5000 });
  });

  test('should show cancel button while streaming', async ({ page }) => {
    // Mock slow response
    await page.route('**/api/chat/message', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 3000));
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
        body: `event: done\ndata: {}\n\n`,
      });
    });

    await sendMessage(page, 'Slow request');

    // Cancel button should appear
    await expect(page.locator('button:has-text("Cancel")')).toBeVisible({ timeout: 2000 });
  });
});

// =============================================================================
// Responsive Design Tests (Desktop only)
// =============================================================================

test.describe('Responsive Design', () => {
  test('should show sidebar on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto('/');

    // New Chat button should be visible (indicates sidebar is visible)
    await expect(page.locator('button:has-text("New Chat")')).toBeVisible();
  });

  test('should have mobile toggle button on small screens', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    // Wait for page to load
    await waitForChatReady(page);

    // There should be a toggle button for mobile
    const toggleButton = page.locator('button').filter({ has: page.locator('svg') }).first();
    await expect(toggleButton).toBeVisible();
  });
});

// =============================================================================
// Accessibility Tests
// =============================================================================

test.describe('Accessibility', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should allow typing in input field', async ({ page }) => {
    const input = page.locator('input[placeholder*="message"]');
    await input.click();
    await input.fill('Test accessibility');

    await expect(input).toHaveValue('Test accessibility');
  });

  test('should clear input on Enter key submit', async ({ page }) => {
    await page.route('**/api/chat/message', async (route) => {
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
        body: `event: start\ndata: {"conversation_id": "test"}\n\n`,
      });
    });

    const input = page.locator('input[placeholder*="message"]');
    await input.fill('Enter test');
    await input.press('Enter');

    // Input should be cleared after Enter
    await expect(input).toHaveValue('');
  });
});
