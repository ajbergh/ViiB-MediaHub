export type ImportedAnalysisKind = 'bars' | 'beats' | 'tatums' | 'sections' | 'segments';
export interface ImportedAnalysisPage {
 songId: string; sourceFingerprint: string; recordingId: string; kind: ImportedAnalysisKind;
 provenance: 'spotify_durable_import' | 'spotify_private_cache'; unverified?: boolean; retrievedAt: string; stale: boolean;
 offset: number; limit: number; totalItems: number;
 items: Array<{ [key: string]: unknown; start?: number; duration?: number; confidence?: number; pitches?: number[]; timbre?: number[] }>;
}
export async function loadImportedAnalysis(songId: string, kind: ImportedAnalysisKind, offset: number): Promise<ImportedAnalysisPage | null> {
 const response = await fetch(`/api/v2/analysis/${encodeURIComponent(songId)}/provider-analysis/${kind}?offset=${offset}&limit=25`, { cache: 'no-store' });
 if (response.status === 404) return null;
 if (!response.ok) throw new Error('Retained analysis unavailable');
 const data = await response.json() as ImportedAnalysisPage;
 if (data.songId !== songId || data.kind !== kind || !['spotify_durable_import','spotify_private_cache'].includes(data.provenance) || data.offset !== offset || data.limit !== 25 || !Number.isInteger(data.totalItems) || data.totalItems < offset || !Array.isArray(data.items) || data.items.length > 25 || data.items.length !== Math.min(25,data.totalItems-offset) || !data.sourceFingerprint) throw new Error('Invalid retained analysis');
 for (const item of data.items) {
  if (!item || typeof item !== 'object') throw new Error('Invalid retained interval');
  for (const key of ['start','duration','confidence'] as const) if(item[key] != null && (!Number.isFinite(item[key]) || item[key]! < 0 || (key==='confidence' && item[key]! > 1))) throw new Error('Invalid retained interval');
  for (const key of ['pitches','timbre'] as const) if(item[key] != null && (!Array.isArray(item[key]) || item[key]!.length !== 12 || item[key]!.some(value=>!Number.isFinite(value)))) throw new Error('Invalid retained vector');
 }
 return data;
}
