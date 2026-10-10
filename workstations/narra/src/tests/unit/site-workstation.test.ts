import { describe, expect, it } from 'vitest';
import { SESSION_SECONDS, fixedSiteProvider, isSessionLive, allowedWorkstationPath } from '../../lib/workstation/policy';

describe('site workstation policy', () => {
  it('expires exactly six hours after entry, without sliding on activity', () => {
    expect(SESSION_SECONDS).toBe(21600);
    expect(isSessionLive(1000, 1000 + 21600 - 1)).toBe(true);
    expect(isSessionLive(1000, 1000 + 21600)).toBe(false);
  });
  it('uses only the server site gateway even if a client changes base URL', () => {
    expect(fixedSiteProvider('http://site-api:8080/v1', 'site-key', 'gpt-image-2'))
      .toEqual({ baseUrl: 'http://site-api:8080/v1', apiKey: 'site-key', model: 'gpt-image-2', models: ['gpt-image-2'], remember: false, label: '本站' });
  });
  it('blocks removed auth, credits, channel and external API routes', () => {
    for (const path of ['/login', '/api/auth/login', '/api/provider-config/save', '/v1/images/generations', '/admin/users', '/api/check-in']) expect(allowedWorkstationPath(path)).toBe(false);
    for (const path of ['/create', '/api/generate', '/api/me/generations/job1', '/api/workstation/media/session/file.png', '/admin/generations']) expect(allowedWorkstationPath(path)).toBe(true);
  });
});
