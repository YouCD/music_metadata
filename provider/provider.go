package provider

import "context"

// SongInfo 统一的歌曲信息结构
type SongInfo struct {
	Title    string // 歌曲名
	Artist   string // 歌手名
	Album    string // 专辑名
	Date     string // 发布日期
	SongID   string // 歌曲唯一 ID
	PicURL   string // 封面图片 URL
	LrcURL   string // 歌词 URL
	Duration int    // 时长（毫秒）
}

// Provider 音乐元数据提供者接口
// 抽象不同音乐 API 的统一调用方式，便于扩展新的数据源
type Provider interface {
	// Search 搜索歌曲，返回匹配的歌曲列表
	Search(ctx context.Context, keyword string) ([]SongInfo, error)

	// GetLyrics 获取歌词内容
	GetLyrics(ctx context.Context, song SongInfo) (string, error)

	// GetCover 获取封面图片数据，返回图片二进制数据和 MIME 类型
	GetCover(ctx context.Context, song SongInfo) ([]byte, string, error)

	// Name 返回提供者名称
	Name() string
}
