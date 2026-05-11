package cmd

import (
	"fmt"
	"music_metadata/metadata"
	"music_metadata/meting"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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
		fmt.Fprintf(os.Stderr, "%s警告: 未找到 ffmpeg，无法嵌入元数据到非 MP3 文件。%s\n", ColorYellow, ColorReset)
		fmt.Fprintf(os.Stderr, "%s建议安装 ffmpeg 或使用 --external 选项保存为独立文件。%s\n\n", ColorYellow, ColorReset)
	}

	// 创建 Meting API 客户端
	client := meting.NewClient(apiBase, server, secretKey)

	fmt.Printf("%s🎵 音乐元数据补全工具%s\n", ColorBold, ColorReset)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  📁 目录:     %s\n", dir)
	fmt.Printf("  🎶 平台:     %s\n", server)
	fmt.Printf("  📝 歌词:     %s\n", boolToStr(!skipLyrics))
	fmt.Printf("  🖼️  封面:     %s\n", boolToStr(!skipCover))
	fmt.Printf("  🔧 模式:     %s\n", modeDisplay())
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// 查找所有音乐文件
	files, err := metadata.FindMusicFiles(dir)
	if err != nil {
		return fmt.Errorf("查找音乐文件失败: %w", err)
	}

	if len(files) == 0 {
		fmt.Printf("%s⚠️  未找到支持的音乐文件%s\n", ColorYellow, ColorReset)
		return nil
	}

	fmt.Printf("找到 %d 个音乐文件\n\n", len(files))

	// 统计
	stats := struct {
		total   int
		success int
		failed  int
	}{total: len(files)}

	// 处理每个文件
	for i, filePath := range files {
		relPath, _ := filepath.Rel(dir, filePath)
		fmt.Printf("[%d/%d] %s处理: %s%s\n", i+1, stats.total, ColorCyan, relPath, ColorReset)

		if err := processFile(filePath, client); err != nil {
			fmt.Printf("  %s❌ 失败: %v%s\n", ColorRed, err, ColorReset)
			stats.failed++
		} else {
			stats.success++
		}
		fmt.Println()
	}

	// 打印汇总
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("%s📊 处理完成%s\n", ColorBold, ColorReset)
	fmt.Printf("  总计: %d  成功: %s%d%s  失败: %s%d%s\n",
		stats.total,
		ColorGreen, stats.success, ColorReset,
		ColorRed, stats.failed, ColorReset,
	)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// processFile 处理单个音乐文件
func processFile(filePath string, client *meting.Client) error {
	// 1. 读取文件现有元数据
	mf, err := metadata.ReadMusicFile(filePath)
	if err != nil {
		fmt.Printf("  %s⚠️  读取元数据失败: %v，尝试从文件名推断%s\n", ColorYellow, err, ColorReset)
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
			fmt.Printf("  💡 从文件名推断: %s - %s\n", mf.Artist, mf.Title)
		} else {
			// 如果无法从文件名推断，使用不带扩展名的文件名作为标题
			name := filepath.Base(filePath)
			name = strings.TrimSuffix(name, filepath.Ext(name))
			mf.Title = name
			fmt.Printf("  💡 使用文件名作为标题: %s\n", mf.Title)
		}
	}

	fmt.Printf("  标题: %s\n", mf.Title)
	fmt.Printf("  歌手: %s\n", mf.Artist)
	fmt.Printf("  专辑: %s\n", mf.Album)
	fmt.Printf("  歌词: %s  封面: %s\n", boolIcon(mf.HasLyrics), boolIcon(mf.HasCover))

	// 2. 构建搜索关键词
	keyword := buildSearchKeyword(mf.Title, mf.Artist)
	fmt.Printf("  🔍 搜索: \"%s\"\n", keyword)

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
	fmt.Printf("  ✅ 匹配: %s - %s (ID: %s)\n", bestMatch.Author, bestMatch.Title, bestMatch.SongID)

	if bestMatch.SongID == "" {
		return fmt.Errorf("无法获取歌曲 ID")
	}

	if dryRun {
		fmt.Printf("  %s🔍 预览模式，不修改文件%s\n", ColorYellow, ColorReset)
		return nil
	}

	// 5. 获取并写入歌词
	if !skipLyrics {
		needLyrics := !mf.HasLyrics || forceUpdate
		if needLyrics {
			writeLyricsFromURL(filePath, client, bestMatch.Lrc)
		} else {
			fmt.Printf("  📝 歌词已存在，跳过\n")
		}
	}

	// 6. 获取并写入封面
	if !skipCover {
		needCover := !mf.HasCover || forceUpdate
		if needCover {
			writeCoverFromURL(filePath, client, bestMatch.Pic)
		} else {
			fmt.Printf("  🖼️  封面已存在，跳过\n")
		}
	}

	return nil
}

// writeLyricsFromURL 从 URL 获取并写入歌词（URL 已包含正确的 auth token）
func writeLyricsFromURL(filePath string, client *meting.Client, lrcURL string) {
	fmt.Printf("  📝 获取歌词...")

	if lrcURL == "" {
		fmt.Printf(" %s歌词 URL 为空%s\n", ColorYellow, ColorReset)
		return
	}

	lyrics, err := client.GetLyricsFromURL(lrcURL)
	if err != nil {
		fmt.Printf(" %s失败: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	if strings.TrimSpace(lyrics) == "" {
		fmt.Printf(" %s歌词为空%s\n", ColorYellow, ColorReset)
		return
	}

	if saveExternal {
		if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
			fmt.Printf(" %s写入 .lrc 失败: %v%s\n", ColorRed, err, ColorReset)
		} else {
			fmt.Printf(" %s✅ 已保存 .lrc 文件%s\n", ColorGreen, ColorReset)
		}
		return
	}

	// MP3 格式可以直接嵌入歌词
	if metadata.IsMP3(filePath) {
		if err := metadata.WriteLyricsToMP3(filePath, lyrics); err != nil {
			fmt.Printf(" %s写入失败: %v%s\n", ColorRed, err, ColorReset)
		} else {
			fmt.Printf(" %s✅ 已嵌入%s\n", ColorGreen, ColorReset)
		}
		return
	}

	// 其他格式尝试使用 ffmpeg 嵌入
	if metadata.SupportsEmbedding() {
		if err := metadata.WriteLyricsWithFFmpeg(filePath, lyrics); err != nil {
			fmt.Printf(" %sffmpeg 写入失败: %v，回退到 .lrc 文件%s\n", ColorYellow, err, ColorReset)
			if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
				fmt.Printf(" %s写入 .lrc 失败: %v%s\n", ColorRed, err, ColorReset)
			} else {
				fmt.Printf(" %s✅ 已保存 .lrc 文件%s\n", ColorGreen, ColorReset)
			}
		} else {
			fmt.Printf(" %s✅ 已嵌入 (via ffmpeg)%s\n", ColorGreen, ColorReset)
		}
		return
	}

	// ffmpeg 不可用，回退到 .lrc 文件
	if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
		fmt.Printf(" %s写入 .lrc 失败: %v%s\n", ColorRed, err, ColorReset)
	} else {
		fmt.Printf(" %s✅ 已保存 .lrc 文件（ffmpeg 不可用）%s\n", ColorGreen, ColorReset)
	}
}

// writeCoverFromURL 从 URL 获取并写入封面（URL 已包含正确的 auth token）
func writeCoverFromURL(filePath string, client *meting.Client, picURL string) {
	fmt.Printf("  🖼️  获取封面...")

	if picURL == "" {
		fmt.Printf(" %s封面 URL 为空%s\n", ColorYellow, ColorReset)
		return
	}

	coverData, mimeType, err := client.DownloadCoverFromURL(picURL)
	if err != nil {
		fmt.Printf(" %s失败: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	if len(coverData) == 0 {
		fmt.Printf(" %s封面数据为空%s\n", ColorYellow, ColorReset)
		return
	}

	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	if saveExternal {
		if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
			fmt.Printf(" %s写入封面文件失败: %v%s\n", ColorRed, err, ColorReset)
		} else {
			fmt.Printf(" %s✅ 已保存封面文件 (%d KB)%s\n", ColorGreen, len(coverData)/1024, ColorReset)
		}
		return
	}

	if metadata.IsMP3(filePath) {
		if err := metadata.WriteCoverToMP3(filePath, coverData, mimeType); err != nil {
			fmt.Printf(" %s写入失败: %v%s\n", ColorRed, err, ColorReset)
		} else {
			fmt.Printf(" %s✅ 已嵌入 (%d KB)%s\n", ColorGreen, len(coverData)/1024, ColorReset)
		}
		return
	}

	if metadata.SupportsEmbedding() {
		if err := metadata.WriteCoverWithFFmpeg(filePath, coverData); err != nil {
			fmt.Printf(" %sffmpeg 写入失败: %v，回退到 .jpg 文件%s\n", ColorYellow, err, ColorReset)
			if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
				fmt.Printf(" %s写入封面文件失败: %v%s\n", ColorRed, err, ColorReset)
			} else {
				fmt.Printf(" %s✅ 已保存封面文件 (%d KB)%s\n", ColorGreen, len(coverData)/1024, ColorReset)
			}
		} else {
			fmt.Printf(" %s✅ 已嵌入 (via ffmpeg, %d KB)%s\n", ColorGreen, len(coverData)/1024, ColorReset)
		}
		return
	}

	// ffmpeg 不可用，回退到 .jpg 文件
	if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
		fmt.Printf(" %s写入封面文件失败: %v%s\n", ColorRed, err, ColorReset)
	} else {
		fmt.Printf(" %s✅ 已保存封面文件 (%d KB, ffmpeg 不可用)%s\n", ColorGreen, len(coverData)/1024, ColorReset)
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
