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
    await expect(page.locator('text=Ask questions')).toBeVisible();
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

  test('should display user message after sending', async ({ page }) => {
    // Mock empty response to avoid API errors
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'chunk', data: { content: 'Hello!' } },
      { event: 'done', data: { conversation_id: 'test-conv', message: { id: '1', role: 'assistant', content: 'Hello!', timestamp: new Date().toISOString() }, success: true, turns: 1, total_tool_calls: 0, done: true } },
    ]);

    await sendMessage(page, 'Hello');

    // User message should appear
    await expect(page.locator('text=Hello').first()).toBeVisible();
  });
});

// =============================================================================
// SSE Streaming Tests
// =============================================================================

test.describe('SSE Streaming', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should show thinking indicator during processing', async ({ page }) => {
    // Mock SSE with thinking event
    await page.route('**/api/chat/message', async (route) => {
      // Simulate delay with streaming
      await route.fulfill({
        status: 200,
        headers: {
          'Content-Type': 'text/event-stream',
          'Cache-Control': 'no-cache',
        },
        body: `event: start\ndata: {"conversation_id": "test-conv"}\n\nevent: thinking\ndata: {"status": "Analyzing request..."}\n\n`,
      });
    });

    await sendMessage(page, 'Test thinking');

    // Should show thinking indicator
    await expect(page.locator('text=Analyzing request...')).toBeVisible({ timeout: 5000 });
  });

  test('should display streamed content chunks', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'chunk', data: { content: 'First ' } },
      { event: 'chunk', data: { content: 'Second ' } },
      { event: 'chunk', data: { content: 'Third' } },
      { event: 'done', data: { conversation_id: 'test-conv', message: { id: '1', role: 'assistant', content: 'First Second Third', timestamp: new Date().toISOString() }, success: true, turns: 1, total_tool_calls: 0, done: true } },
    ]);

    await sendMessage(page, 'Stream test');

    // Final content should be visible
    await expect(page.locator('text=First Second Third')).toBeVisible({ timeout: 5000 });
  });
});

// =============================================================================
// Tool Call Display Tests
// =============================================================================

test.describe('Tool Call Display', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForChatReady(page);
  });

  test('should display tool call card when tool is used', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'chunk', data: { content: 'Let me read that file...' } },
      { event: 'tool_call', data: { name: 'read_file', input: { path: '/test/file.txt' } } },
      { event: 'tool_result', data: { name: 'read_file', result: 'File content here', success: true, duration_ms: 150 } },
      { event: 'chunk', data: { content: '\nDone reading!' } },
      { event: 'done', data: { conversation_id: 'test-conv', message: { id: '1', role: 'assistant', content: 'Let me read that file...\nDone reading!', timestamp: new Date().toISOString(), tool_calls: [{ name: 'read_file', input: { path: '/test/file.txt' }, result: 'File content here', success: true, duration_ms: 150 }] }, success: true, turns: 1, total_tool_calls: 1, done: true } },
    ]);

    await sendMessage(page, 'Read a file');

    // Tool card should be visible
    await expect(page.locator('text=read_file')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('text=150ms')).toBeVisible({ timeout: 5000 });
  });

  test('should toggle tool call details on click', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'tool_call', data: { name: 'write_file', input: { path: '/test/output.txt', content: 'Hello World' } } },
      { event: 'tool_result', data: { name: 'write_file', result: 'File written', success: true, duration_ms: 100 } },
      { event: 'done', data: { conversation_id: 'test-conv', message: { id: '1', role: 'assistant', content: '', timestamp: new Date().toISOString(), tool_calls: [{ name: 'write_file', input: { path: '/test/output.txt', content: 'Hello World' }, result: 'File written', success: true, duration_ms: 100 }] }, success: true, turns: 1, total_tool_calls: 1, done: true } },
    ]);

    await sendMessage(page, 'Write file');

    // Wait for tool card
    await expect(page.locator('text=write_file')).toBeVisible({ timeout: 5000 });

    // Click to expand
    await page.click('text=write_file');

    // Input should be visible
    await expect(page.locator('text=Input:')).toBeVisible();
    await expect(page.locator('text=/test/output.txt')).toBeVisible();
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

  test('should add conversation to sidebar after sending message', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'conv-123' } },
      { event: 'chunk', data: { content: 'Hi there!' } },
      { event: 'done', data: { conversation_id: 'conv-123', message: { id: '1', role: 'assistant', content: 'Hi there!', timestamp: new Date().toISOString() }, success: true, turns: 1, total_tool_calls: 0, done: true } },
    ]);

    await sendMessage(page, 'My first message');

    // Sidebar should show conversation count (may be "1 conversation")
    await expect(page.locator('text=1 conversation')).toBeVisible({ timeout: 5000 });
  });

  test('should start new conversation when clicking New Chat', async ({ page }) => {
    // First send a message
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'conv-1' } },
      { event: 'chunk', data: { content: 'Response' } },
      { event: 'done', data: { conversation_id: 'conv-1', message: { id: '1', role: 'assistant', content: 'Response', timestamp: new Date().toISOString() }, success: true, turns: 1, total_tool_calls: 0, done: true } },
    ]);

    await sendMessage(page, 'First conversation');
    await expect(page.locator('text=Response')).toBeVisible({ timeout: 5000 });

    // Click New Chat
    await page.click('button:has-text("New Chat")');

    // Should show welcome message again
    await expect(page.locator('text=Welcome to C4 Chat')).toBeVisible();
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
    await expect(page.locator('text=Error: Something went wrong')).toBeVisible({ timeout: 5000 });
  });

  test('should show cancel button during streaming', async ({ page }) => {
    // Mock slow response
    await page.route('**/api/chat/message', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 5000));
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
// Responsive Design Tests
// =============================================================================

test.describe('Responsive Design', () => {
  test('should hide sidebar on mobile by default', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 }); // iPhone SE
    await page.goto('/');

    // Sidebar should be hidden (translated off-screen)
    const sidebar = page.locator('[class*="w-64"]');
    await expect(sidebar).toHaveCSS('transform', /matrix.*-256/); // -translate-x-full
  });

  test('should show toggle button on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    // Toggle button should be visible
    await expect(page.locator('button.lg\\:hidden')).toBeVisible();
  });

  test('should toggle sidebar on mobile when clicking toggle', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    // Click toggle
    await page.click('button.lg\\:hidden');

    // Sidebar should be visible
    await expect(page.locator('button:has-text("New Chat")')).toBeVisible();
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

  test('should have proper focus management', async ({ page }) => {
    // Tab to input
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');

    // Input should be focusable
    const input = page.locator('input[placeholder*="message"]');
    await expect(input).toBeFocused();
  });

  test('should submit on Enter key', async ({ page }) => {
    await mockSSEResponse(page, [
      { event: 'start', data: { conversation_id: 'test-conv' } },
      { event: 'chunk', data: { content: 'Response' } },
      { event: 'done', data: { conversation_id: 'test-conv', message: { id: '1', role: 'assistant', content: 'Response', timestamp: new Date().toISOString() }, success: true, turns: 1, total_tool_calls: 0, done: true } },
    ]);

    const input = page.locator('input[placeholder*="message"]');
    await input.fill('Enter test');
    await page.keyboard.press('Enter');

    // Message should be sent
    await expect(page.locator('text=Enter test')).toBeVisible();
  });
});
