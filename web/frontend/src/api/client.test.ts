import { describe, it, expect, beforeEach, vi } from 'vitest';
import { api } from './client';

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => { store[key] = value; },
    removeItem: (key: string) => { delete store[key]; },
    clear: () => { store = {}; },
  };
})();

Object.defineProperty(window, 'localStorage', { value: localStorageMock });

describe('ApiClient', () => {
  beforeEach(() => {
    localStorageMock.clear();
    api.clearToken();
  });

  describe('token management', () => {
    it('stores token in localStorage when setToken is called', () => {
      api.setToken('test-token');
      expect(localStorage.getItem('auth_token')).toBe('test-token');
    });

    it('retrieves token from localStorage when getToken is called', () => {
      localStorage.setItem('auth_token', 'stored-token');
      expect(api.getToken()).toBe('stored-token');
    });

    it('clears token from localStorage when clearToken is called', () => {
      api.setToken('test-token');
      api.clearToken();
      expect(localStorage.getItem('auth_token')).toBeNull();
      expect(api.getToken()).toBeNull();
    });

    it('caches token in memory after first getToken call', () => {
      localStorage.setItem('auth_token', 'cached-token');
      api.getToken(); // First call - reads from localStorage
      localStorage.removeItem('auth_token'); // Remove from storage
      expect(api.getToken()).toBe('cached-token'); // Should still return cached value
    });
  });

  describe('request handling', () => {
    beforeEach(() => {
      api.setToken('test-token');
    });

    it('throws error when not authenticated', async () => {
      api.clearToken();
      await expect(api.getStatus()).rejects.toThrow('Not authenticated');
    });

    it('makes request with correct authorization header', async () => {
      const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
        new Response(JSON.stringify({ server_name: 'test' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      );

      await api.getStatus();

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/status',
        expect.objectContaining({
          headers: expect.objectContaining({
            'Authorization': 'Bearer test-token',
          }),
        })
      );

      fetchSpy.mockRestore();
    });

    it('clears token on 401 response', async () => {
      api.setToken('invalid-token');

      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
        new Response('Unauthorized', { status: 401 })
      );

      await expect(api.getStatus()).rejects.toThrow('Unauthorized');
      expect(api.getToken()).toBeNull();
    });

    it('throws error on non-ok response', async () => {
      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
        new Response('Server Error', { status: 500 })
      );

      await expect(api.getStatus()).rejects.toThrow('API error: 500');
    });
  });
});
