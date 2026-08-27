package handler

import "strconv"

// parseLimit はクエリパラメータ`limit`を読み取り、[1, max]の範囲にクランプする
// （通知・コメント・ハイライトのページネーションで共通利用、2026-08-27追加）。
// 不正な値（数値でない・0以下等）はdefaultValueにフォールバックする（400にはしない、
// 表示件数調整程度の軽いパラメータのため）。
func parseLimit(limitParam string, defaultValue, max int) int {
	if limitParam == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(limitParam)
	if err != nil || n <= 0 {
		return defaultValue
	}
	if n > max {
		return max
	}
	return n
}
