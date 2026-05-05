package crypto

import "like-a-ethereum/internal/util"

// HashJSON はJSONシリアライズしてSHA-256ハッシュを返す。
// util.HashJSON の薄いラッパー。crypto パッケージ経由でも呼べるようにしている。
var HashJSON = util.HashJSON
