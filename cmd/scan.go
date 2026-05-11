package cmd

import (
	"context"
	"fmt"
	"music_metadata/metadata"
	"music_metadata/meting"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/youcd/toolkit/log"
)

var (
	skipLyrics   bool
	skipCover    bool
	saveExternal bool
)

var scanCmd = &cobra.Command{
	Use:   "scan [目录路径]",
	Short: "扫描目录并补全音乐元数据",
	Long: fmt.Sprintf("%s扫描目录并补全音乐元数据%s\n\n"+
		"递归扫描指定目录中的音乐文件，通过 Meting API 搜索匹配的歌曲，\n"+
		"自动获取并嵌入歌词和封面图片到音频文件中。\n\n"+
		"%s示例:%s\n"+
		"  music_metadata scan ./music\n"+
		"  music_metadata scan ./music -s tencent\n"+
		"  music_metadata scan ./music --dry-run\n"+
		"  music_metadata scan ./music --external\n"+
		"  music_metadata scan ./music --no-lyrics --force\n",
		ColorBold, ColorReset,
		ColorCyan, ColorReset,
	),
	Args: cobra.MaximumNArgs(1),
	RunE: runScan,
}

func init() {
	scanCmd.Flags().BoolVar(&skipLyrics, "no-lyrics", false, "不获取歌词")
	scanCmd.Flags().BoolVar(&skipCover, "no-cover", false, "不获取封面")
	scanCmd.Flags().BoolVar(&saveExternal, "external", false, "保存为独立的 .lrc/.jpg 文件（不嵌入音频文件）")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("目录不存在: %s", dir)
	}

	// 检查 ffmpeg 是否可用（非 external 模式时）
	if !saveExternal && !metadata.SupportsEmbedding() {
		log.WithCtx(cmd.Context()).Warn("未找到 ffmpeg，无法嵌入元数据到非 MP3 文件。建议安装 ffmpeg 或使用 --external 选项保存为独立文件")
	}

	// 创建 Meting API 客户端
	client := meting.NewClient(apiBase, server, secretKey)

	log.WithCtx(cmd.Context()).Infof("🎵 音乐元数据补全工具 - 目录: %s, 平台: %s, 歌词: %s, 封面: %s, 模式: %s",
		dir, server, boolToStr(!skipLyrics), boolToStr(!skipCover), modeDisplay())

	// 查找所有音乐文件
	files, err := metadata.FindMusicFiles(dir)
	if err != nil {
		return fmt.Errorf("查找音乐文件失败: %w", err)
	}

	if len(files) == 0 {
		log.WithCtx(cmd.Context()).Warn("⚠️  未找到支持的音乐文件")
		return nil
	}

	log.WithCtx(cmd.Context()).Info(fmt.Sprintf("找到 %d 个音乐文件", len(files)))

	// 统计
	stats := struct {
		total   int
		success int
		failed  int
	}{total: len(files)}

	// 处理每个文件
	for i, filePath := range files {
		relPath, _ := filepath.Rel(dir, filePath)
		log.WithCtx(cmd.Context()).Info(fmt.Sprintf("[%d/%d] 处理: %s", i+1, stats.total, relPath))

		if err := processFile(filePath, client, cmd.Context()); err != nil {
			log.WithCtx(cmd.Context()).Error(fmt.Sprintf("❌ 失败: %v", err))
			stats.failed++
		} else {
			stats.success++
		}
	}

	// 打印汇总
	log.WithCtx(cmd.Context()).Infof("📊 处理完成 - 总计: %d, 成功: %d, 失败: %d",
		stats.total, stats.success, stats.failed)

	return nil
}

// processFile 处理单个音乐文件
func processFile(filePath string, client *meting.Client, ctx context.Context) error {
	// 1. 读取文件现有元数据
	mf, err := metadata.ReadMusicFile(filePath)
	if err != nil {
		log.WithCtx(ctx).Warn(fmt.Sprintf("⚠️  读取元数据失败: %v，尝试从文件名推断", err))
		mf = guessFromFilename(filePath)
	}

	// 如果标题为空，尝试从文件名推断
	if mf.Title == "" {
		guessed := guessFromFilename(filePath)
		if guessed.Title != "" {
			mf.Title = guessed.Title
			// 如果原来没有歌手信息，也尝试从文件名获取
			if mf.Artist == "" && guessed.Artist != "" {
				mf.Artist = guessed.Artist
			}
			log.WithCtx(ctx).Info(fmt.Sprintf("💡 从文件名推断: %s - %s", mf.Artist, mf.Title))
		} else {
			// 如果无法从文件名推断，使用不带扩展名的文件名作为标题
			name := filepath.Base(filePath)
			name = strings.TrimSuffix(name, filepath.Ext(name))
			mf.Title = name
			log.WithCtx(ctx).Info(fmt.Sprintf("💡 使用文件名作为标题: %s", mf.Title))
		}
	}

	log.WithCtx(ctx).Infof("文件信息 - 标题: %s, 歌手: %s, 专辑: %s, 有歌词: %v, 有封面: %v",
		mf.Title, mf.Artist, mf.Album, mf.HasLyrics, mf.HasCover)

	// 2. 构建搜索关键词
	keyword := buildSearchKeyword(mf.Title, mf.Artist)
	log.WithCtx(ctx).Info(fmt.Sprintf("🔍 搜索: \"%s\"", keyword))

	// 3. 搜索歌曲
	songs, err := client.Search(keyword)
	if err != nil {
		return fmt.Errorf("搜索失败: %w", err)
	}

	if len(songs) == 0 {
		return fmt.Errorf("未找到匹配的歌曲")
	}

	// 4. 选择最佳匹配
	bestMatch := findBestMatch(songs, mf.Title, mf.Artist)
	log.WithCtx(ctx).Info(fmt.Sprintf("✅ 匹配: %s - %s (ID: %s)", bestMatch.Author, bestMatch.Title, bestMatch.SongID))

	if bestMatch.SongID == "" {
		return fmt.Errorf("无法获取歌曲 ID")
	}

	if dryRun {
		log.WithCtx(ctx).Info("🔍 预览模式，不修改文件")
		return nil
	}

	// 5. 获取并写入歌词
	if !skipLyrics {
		needLyrics := !mf.HasLyrics || forceUpdate
		if needLyrics {
			writeLyricsFromURL(filePath, client, bestMatch.Lrc, ctx)
		} else {
			log.WithCtx(ctx).Info("📝 歌词已存在，跳过")
		}
	}

	// 6. 获取并写入封面
	if !skipCover {
		needCover := !mf.HasCover || forceUpdate
		if needCover {
			writeCoverFromURL(filePath, client, bestMatch.Pic, ctx)
		} else {
			log.WithCtx(ctx).Info("🖼️  封面已存在，跳过")
		}
	}

	return nil
}

// writeLyricsFromURL 从 URL 获取并写入歌词（URL 已包含正确的 auth token）
func writeLyricsFromURL(filePath string, client *meting.Client, lrcURL string, ctx context.Context) {
	log.WithCtx(ctx).Info("📝 获取歌词...")

	if lrcURL == "" {
		log.WithCtx(ctx).Warn("歌词 URL 为空")
		return
	}

	lyrics, err := client.GetLyricsFromURL(lrcURL)
	if err != nil {
		log.WithCtx(ctx).Error(fmt.Sprintf("失败: %v", err))
		return
	}
	if strings.TrimSpace(lyrics) == "" {
		log.WithCtx(ctx).Warn("歌词为空")
		return
	}

	if saveExternal {
		if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
			log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
		} else {
			log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件")
		}
		return
	}

	// MP3 格式可以直接嵌入歌词
	if metadata.IsMP3(filePath) {
		if err := metadata.WriteLyricsToMP3(filePath, lyrics); err != nil {
			log.WithCtx(ctx).Error(fmt.Sprintf("写入失败: %v", err))
		} else {
			log.WithCtx(ctx).Info("✅ 已嵌入")
		}
		return
	}

	// 其他格式尝试使用 ffmpeg 嵌入
	if metadata.SupportsEmbedding() {
		if err := metadata.WriteLyricsWithFFmpeg(filePath, lyrics); err != nil {
			log.WithCtx(ctx).Warn(fmt.Sprintf("ffmpeg 写入失败: %v，回退到 .lrc 文件", err))
			if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
			} else {
				log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件")
			}
		} else {
			log.WithCtx(ctx).Info("✅ 已嵌入 (via ffmpeg)")
		}
		return
	}

	// ffmpeg 不可用，回退到 .lrc 文件
	if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
		log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
	} else {
		log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件（ffmpeg 不可用）")
	}
}

// writeCoverFromURL 从 URL 获取并写入封面（URL 已包含正确的 auth token）
func writeCoverFromURL(filePath string, client *meting.Client, picURL string, ctx context.Context) {
	log.WithCtx(ctx).Info("🖼️  获取封面...")

	if picURL == "" {
		log.WithCtx(ctx).Warn("封面 URL 为空")
		return
	}

	coverData, mimeType, err := client.DownloadCoverFromURL(picURL)
	if err != nil {
		log.WithCtx(ctx).Error(fmt.Sprintf("失败: %v", err))
		return
	}
	if len(coverData) == 0 {
		log.WithCtx(ctx).Warn("封面数据为空")
		return
	}

	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	if saveExternal {
		if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
			log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
		} else {
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB)", len(coverData)/1024))
		}
		return
	}

	if metadata.IsMP3(filePath) {
		if err := metadata.WriteCoverToMP3(filePath, coverData, mimeType); err != nil {
			log.WithCtx(ctx).Error(fmt.Sprintf("写入失败: %v", err))
		} else {
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已嵌入 (%d KB)", len(coverData)/1024))
		}
		return
	}

	if metadata.SupportsEmbedding() {
		if err := metadata.WriteCoverWithFFmpeg(filePath, coverData); err != nil {
			log.WithCtx(ctx).Warn(fmt.Sprintf("ffmpeg 写入失败: %v，回退到 .jpg 文件", err))
			if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
			} else {
				log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB)", len(coverData)/1024))
			}
		} else {
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已嵌入 (via ffmpeg, %d KB)", len(coverData)/1024))
		}
		return
	}

	// ffmpeg 不可用，回退到 .jpg 文件
	if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
		log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
	} else {
		log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB, ffmpeg 不可用)", len(coverData)/1024))
	}
}

// guessFromFilename 从文件名猜测标题和歌手
// 常见格式: "歌手 - 标题.flac"、"歌手-标题.flac"、"01 - 标题.flac"、"歌手 - 专辑 - 标题.flac"
func guessFromFilename(filePath string) *metadata.MusicFile {
	name := filepath.Base(filePath)
	name = strings.TrimSuffix(name, filepath.Ext(name))

	// 去除音轨号前缀（如 "01 - " 或 "01. "）
	name = strings.TrimLeft(name, "0123456789")
	name = strings.TrimLeft(name, ". -_")
	name = strings.TrimSpace(name)

	// 优先尝试用 " - " 分割（有空格），最多分成2部分（歌手 - 标题）
	parts := strings.SplitN(name, " - ", 2)

	// 如果只有一部分，尝试用 "-" 分割（没有空格）
	if len(parts) == 1 {
		parts = strings.SplitN(name, "-", 2)
	}

	switch len(parts) {
	case 2:
		return &metadata.MusicFile{
			FilePath: filePath,
			Artist:   strings.TrimSpace(parts[0]),
			Title:    strings.TrimSpace(parts[1]),
		}
	default:
		// 无法解析，使用整个文件名作为标题
		return &metadata.MusicFile{
			FilePath: filePath,
			Title:    name,
		}
	}
}

// buildSearchKeyword 构建搜索关键词
func buildSearchKeyword(title, artist string) string {
	if title != "" && artist != "" {
		return fmt.Sprintf("%s %s", artist, title)
	}
	if title != "" {
		return title
	}
	// 如果只有艺术家没有标题
	if artist != "" {
		return artist
	}
	return ""
}

// findBestMatch 从搜索结果中选择最佳匹配
func findBestMatch(songs []meting.SongInfo, title, artist string) meting.SongInfo {
	if len(songs) == 0 {
		return meting.SongInfo{}
	}

	bestScore := -1
	bestIdx := 0

	titleLower := strings.ToLower(title)
	artistLower := strings.ToLower(artist)

	for i, song := range songs {
		score := 0
		songTitle := strings.ToLower(song.Title)
		songAuthor := strings.ToLower(song.Author)

		// 标题完全匹配
		if songTitle == titleLower {
			score += 100
		} else if strings.Contains(songTitle, titleLower) || strings.Contains(titleLower, songTitle) {
			score += 50
		}

		// 歌手匹配
		if songAuthor == artistLower {
			score += 80
		} else if strings.Contains(songAuthor, artistLower) || strings.Contains(artistLower, songAuthor) {
			score += 40
		}

		// 有 songID 加分
		if song.SongID != "" {
			score += 10
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return songs[bestIdx]
}

// 辅助函数
func boolToStr(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

func boolIcon(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

func modeDisplay() string {
	if dryRun {
		return "预览模式（不修改文件）"
	}
	if saveExternal {
		return "外部文件模式（保存为 .lrc/.jpg）"
	}
	return "嵌入模式"
}
