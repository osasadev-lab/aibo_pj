// @opennextjs/cloudflareが生成したデフォルトのopen-next.config.ts
import { defineCloudflareConfig } from "@opennextjs/cloudflare";
// import r2IncrementalCache from "@opennextjs/cloudflare/overrides/incremental-cache/r2-incremental-cache";

export default defineCloudflareConfig({
	// キャッシュ精度を上げたい場合はR2キャッシュの有効化を検討する
	// 詳細: https://opennext.js.org/cloudflare/caching
	// incrementalCache: r2IncrementalCache
});
