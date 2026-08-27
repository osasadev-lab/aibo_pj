package handler

import "github.com/google/uuid"

// dedupUUIDs は重複を除いたUUID列を返す（順序は最初の出現順を維持する）。
// 担当者・タグ・メンション先など複数のハンドラで共通利用する。
func dedupUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
