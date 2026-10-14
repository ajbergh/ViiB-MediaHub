package db

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
)

// CompactLegacyThreeBandArtifacts rewrites a bounded batch without decoding audio.
// The compare-and-swap protects concurrent analysis and source replacement. Bad
// legacy payloads remain available for the existing repair path to diagnose.
// Freed pages are reusable; physical file compaction is a separate operation.
func (d *DB) CompactLegacyThreeBandArtifacts(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return 0, err
	}
	rows, err := d.conn.QueryContext(ctx, `SELECT id FROM track_analysis_artifacts
        WHERE kind=? AND format_version=? AND algorithm_version=? AND encoding=?
        AND length(data) BETWEEN 1 AND ? ORDER BY id LIMIT ?`,
		threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion, threeband.LegacyEncoding, threeband.MaxBytes, min(limit, 128))
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	var changed int64
	for _, id := range ids {
		var data []byte
		err := d.conn.QueryRowContext(ctx, `SELECT CASE WHEN length(data)<=? THEN data ELSE NULL END
            FROM track_analysis_artifacts WHERE id=? AND encoding=?`, threeband.MaxBytes, id, threeband.LegacyEncoding).Scan(&data)
		if err != nil {
			if ctx.Err() != nil {
				return changed, ctx.Err()
			}
			continue
		}
		overview, err := threeband.Decode(data)
		if err != nil {
			continue
		}
		compact, err := threeband.Encode(overview)
		if err != nil || len(compact) >= len(data) {
			continue
		}
		result, err := d.conn.ExecContext(ctx, `UPDATE track_analysis_artifacts SET encoding=?,data=?
            WHERE id=? AND encoding=? AND data=?`, threeband.Encoding, compact, id, threeband.LegacyEncoding, data)
		if err != nil {
			return changed, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return changed, err
		}
		changed += n
	}
	return changed, nil
}
