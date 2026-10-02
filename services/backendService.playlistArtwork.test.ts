import { afterEach, expect, it, vi } from 'vitest';
import { backendService } from './backendService';
import { api } from './api';
afterEach(() => vi.restoreAllMocks());
it('retains existing playlist artwork when track membership is saved', async () => {
  vi.spyOn(api, 'healthCheck').mockResolvedValue({} as never);
  const update = vi.spyOn(api, 'updatePlaylist').mockResolvedValue({} as never);
  await backendService.updatePlaylist({ id: 'playlist', name: 'Playlist', songIds: ['a', 'b'], coverUrl: '/custom.jpg', createdAt: 1 });
  expect(update).toHaveBeenCalledWith('playlist', expect.objectContaining({ songIds: ['a', 'b'], coverPath: '/custom.jpg' }));
});
