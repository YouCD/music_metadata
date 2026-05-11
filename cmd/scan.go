package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/YouCD/music_metadata/metadata"
	"github.com/YouCD/music_metadata/provider"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/youcd/toolkit/log"
)

var (
	skipLyrics   bool
	skipCover    bool
	saveExternal bool
	workers      int
)

var scanCmd = &cobra.Command{
	Use:   "scan [目录路径]",
	Short: "扫描目录并补全音乐元数据",
	Long: fmt.Sprintf("%s扫描目录并补全音乐元数据%s\n\n"+
		"递归扫描指定目录中的音乐文件，通过音乐 API 搜索匹配的歌曲，\n"+
		"自动获取并嵌入歌词和封面图片到音频文件中。\n\n"+
		"%s示例:%s\n"+
		"  music_metadata scan ./music\n"+
		"  music_metadata scan ./music -p netease\n"+
		"  music_metadata scan ./music --dry-run\n"+
		"  music_metadata scan ./music --external\n"+
		"  music_metadata scan ./music --no-lyrics --force\n"+
		"  music_metadata scan ./music -w 5\n",
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
	scanCmd.Flags().IntVarP(&workers, "workers", "w", 10, "并发处理数")
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

	// 创建元数据提供者
	p, err := newProvider(providerName, apiBase)
	if err != nil {
		return fmt.Errorf("创建提供者失败: %w", err)
	}

	log.WithCtx(cmd.Context()).Infof("🎵 音乐元数据补全工具 - 目录: %s, 提供者: %s, 歌词: %s, 封面: %s, 模式: %s",
		dir, p.Name(), boolToStr(!skipLyrics), boolToStr(!skipCover), modeDisplay())

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

	// 统计（使用原子操作保证并发安全）
	var successCount int32
	var failedCount int32

	// 创建进度条，期间将日志级别设为 error，避免日志干扰进度条显示
	log.SetLogLevel("error")

	progressBar, err := pterm.DefaultProgressbar.WithTotal(len(files)).WithTitle("处理音乐文件").Start()
	if err != nil {
		log.SetLogLevel("info")
		log.WithCtx(cmd.Context()).Error(fmt.Sprintf("创建进度条失败: %v", err))
	}

	// 使用 semaphore 控制并发数
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, filePath := range files {
		wg.Add(1)
		sem <- struct{}{} // 获取信号量

		go func(fp string) {
			defer wg.Done()
			defer func() { <-sem }() // 释放信号量

			relPath, _ := filepath.Rel(dir, fp)

			if err := processFile(fp, p, cmd.Context()); err != nil {
				atomic.AddInt32(&failedCount, 1)
				_ = relPath // 并发模式下不打印错误日志，避免干扰进度条
			} else {
				atomic.AddInt32(&successCount, 1)
			}

			if progressBar != nil {
				progressBar.UpdateTitle(relPath)
				progressBar.Increment()
			}
		}(filePath)
	}

	// 等待所有任务完成
	wg.Wait()

	// 恢复日志级别
	log.SetLogLevel("info")

	// 打印汇总
	pterm.Info.Printfln("📊 处理完成 - 总计: %d, 成功: %d, 失败: %d",
		len(files), successCount, failedCount)

	return nil
}

// processFile 处理单个音乐文件
func processFile(filePath string, p provider.Provider, ctx context.Context) error {
	// 1. 读取文件现有元数据
	mf, err := metadata.ReadMusicFile(filePath)
	if err != nil {
		// dhowden/tag 读取失败，尝试回退方案
		if metadata.IsWAV(filePath) {
			// WAV：纯 Go 解析 RIFF INFO 元数据
			if wavMf, wavErr := metadata.ReadMusicFileFromWAV(filePath); wavErr == nil {
				mf = wavMf
				err = nil
			}
		} else if metadata.IsAPE(filePath) {
			// APE：使用 ffprobe 读取元数据
			if apeMf, apeErr := metadata.ReadMusicFileWithFFprobe(filePath); apeErr == nil {
				mf = apeMf
				err = nil
			}
		}
		if err != nil {
			log.WithCtx(ctx).Warn(fmt.Sprintf("⚠️  读取元数据失败: %v，尝试从文件名推断", err))
			mf = guessFromFilename(filePath)
		}
	}

	// 从文件名推断信息，用于补充缺失的元数据和优化搜索
	guessed := guessFromFilename(filePath)

	// 如果标题为空，使用文件名推断的标题
	if mf.Title == "" {
		if guessed.Title != "" {
			mf.Title = guessed.Title
			log.WithCtx(ctx).Info(fmt.Sprintf("💡 从文件名推断标题: %s", mf.Title))
		} else {
			// 如果无法从文件名推断，使用不带扩展名的文件名作为标题
			name := filepath.Base(filePath)
			name = strings.TrimSuffix(name, filepath.Ext(name))
			mf.Title = name
			log.WithCtx(ctx).Info(fmt.Sprintf("💡 使用文件名作为标题: %s", mf.Title))
		}
	}

	// 如果歌手为空，使用文件名推断的歌手
	if mf.Artist == "" && guessed.Artist != "" {
		mf.Artist = guessed.Artist
		log.WithCtx(ctx).Info(fmt.Sprintf("💡 从文件名推断歌手: %s", mf.Artist))
	}

	log.WithCtx(ctx).Infof("文件信息 - 标题: %s, 歌手: %s, 专辑: %s, 有歌词: %v, 有封面: %v",
		mf.Title, mf.Artist, mf.Album, mf.HasLyrics, mf.HasCover)

	// 检查元信息是否完整，完整则跳过（非强制更新模式下）
	if !forceUpdate && mf.IsComplete() {
		log.WithCtx(ctx).Info("✅ 元信息完整，跳过")
		return nil
	}

	// 2. 构建搜索关键词
	// 优先使用文件名中推断的歌手信息，因为文件名通常更准确
	searchArtist := mf.Artist
	if guessed.Artist != "" {
		searchArtist = guessed.Artist
	}
	keyword := buildSearchKeyword(mf.Title, searchArtist)
	log.WithCtx(ctx).Info(fmt.Sprintf("🔍 搜索: \"%s\"", keyword))

	// 3. 搜索歌曲
	songs, err := p.Search(ctx, keyword)
	if err != nil {
		return fmt.Errorf("搜索失败: %w", err)
	}

	if len(songs) == 0 {
		return fmt.Errorf("未找到匹配的歌曲")
	}

	// 4. 选择最佳匹配（使用文件名推断的歌手信息进行匹配）
	bestMatch := findBestMatch(songs, mf.Title, searchArtist)
	log.WithCtx(ctx).Info(fmt.Sprintf("✅ 匹配: %s - %s (ID: %s)", bestMatch.Artist, bestMatch.Title, bestMatch.SongID))

	if bestMatch.SongID == "" {
		return fmt.Errorf("无法获取歌曲 ID")
	}

	if dryRun {
		log.WithCtx(ctx).Info("🔍 预览模式，不修改文件")
		return nil
	}

	// 5. 获取歌词和封面数据
	var lyrics string
	var coverData []byte
	var mimeType string

	if !skipLyrics {
		needLyrics := !mf.HasLyrics || forceUpdate
		if needLyrics {
			log.WithCtx(ctx).Info("📝 获取歌词...")
			l, err := p.GetLyrics(ctx, bestMatch)
			if err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("获取歌词失败: %v", err))
			} else if strings.TrimSpace(l) == "" {
				log.WithCtx(ctx).Warn("歌词为空")
			} else {
				lyrics = l
			}
		} else {
			log.WithCtx(ctx).Info("📝 歌词已存在，跳过")
		}
	}

	if !skipCover {
		needCover := !mf.HasCover || forceUpdate
		if needCover {
			log.WithCtx(ctx).Info("🖼️  获取封面...")
			data, mime, err := p.GetCover(ctx, bestMatch)
			if err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("获取封面失败: %v", err))
			} else if len(data) == 0 {
				log.WithCtx(ctx).Warn("封面数据为空")
			} else {
				coverData = data
				mimeType = mime
			}
		} else {
			log.WithCtx(ctx).Info("🖼️  封面已存在，跳过")
		}
	}

	// 6. 同时写入元数据（title、artist、album、date、歌词、封面）
	if bestMatch.Title != "" || lyrics != "" || len(coverData) > 0 || bestMatch.Artist != "" || bestMatch.Album != "" || bestMatch.Date != "" {
		writeMetadata(filePath, bestMatch.Title, bestMatch.Artist, bestMatch.Album, bestMatch.Date, lyrics, coverData, mimeType, ctx)
	}

	return nil
}

// writeMetadata 同时写入元数据（title、artist、album、date、歌词、封面）
func writeMetadata(filePath, title, artist, album, date, lyrics string, coverData []byte, mimeType string, ctx context.Context) {
	if saveExternal {
		// 保存为外部文件
		if lyrics != "" {
			if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
			} else {
				log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件")
			}
		}
		if len(coverData) > 0 {
			if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
			} else {
				log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB)", len(coverData)/1024))
			}
		}
		return
	}

	// WAV 格式：只支持基础元数据嵌入，歌词和封面保存为外部文件
	if metadata.IsWAV(filePath) {
		// 写入基础元数据（标题、歌手、专辑、日期）
		if title != "" || artist != "" || album != "" || date != "" {
			if metadata.SupportsEmbedding() {
				err := metadata.WriteAllWithFFmpeg(filePath, title, artist, album, date, "", nil, "")
				if err != nil {
					log.WithCtx(ctx).Error(fmt.Sprintf("写入 WAV 元数据失败: %v", err))
				} else {
					log.WithCtx(ctx).Info("✅ 已嵌入基础元数据（标题/歌手/专辑/日期 via ffmpeg）")
				}
			}
		}
		// 歌词和封面保存为外部文件
		if lyrics != "" {
			if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
			} else {
				log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件（WAV 不支持嵌入歌词）")
			}
		}
		if len(coverData) > 0 {
			if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
			} else {
				log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB)（WAV 不支持嵌入封面）", len(coverData)/1024))
			}
		}
		return
	}

	// APE 格式：ffmpeg 不支持 APE muxer，所有元数据保存为外部文件
	if metadata.IsAPE(filePath) {
		if lyrics != "" {
			if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
			} else {
				log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件（APE 不支持嵌入歌词）")
			}
		}
		if len(coverData) > 0 {
			if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
				log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
			} else {
				log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB)（APE 不支持嵌入封面）", len(coverData)/1024))
			}
		}
		return
	}

	// MP3 格式使用 id3v2 库
	if metadata.IsMP3(filePath) {
		var err error
		if title != "" || lyrics != "" || len(coverData) > 0 || artist != "" || album != "" || date != "" {
			err = metadata.WriteAllToMP3(filePath, title, artist, album, date, lyrics, coverData, mimeType)
		}

		if err != nil {
			log.WithCtx(ctx).Error(fmt.Sprintf("写入失败: %v", err))
		} else {
			log.WithCtx(ctx).Info("✅ 已嵌入元数据（标题/歌手/专辑/歌词/封面）")
		}
		return
	}

	// 其他格式使用 ffmpeg
	if metadata.SupportsEmbedding() {
		err := metadata.WriteAllWithFFmpeg(filePath, title, artist, album, date, lyrics, coverData, mimeType)

		if err != nil {
			log.WithCtx(ctx).Warn(fmt.Sprintf("ffmpeg 写入失败: %v，回退到外部文件", err))
			// 回退到外部文件
			if lyrics != "" {
				if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
					log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
				} else {
					log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件")
				}
			}
			if len(coverData) > 0 {
				if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
					log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
				} else {
					log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB)", len(coverData)/1024))
				}
			}
		} else {
			log.WithCtx(ctx).Info("✅ 已嵌入元数据（标题/歌手/专辑/日期/歌词/封面 via ffmpeg）")
		}
		return
	}

	// ffmpeg 不可用，回退到外部文件
	if lyrics != "" {
		if err := metadata.WriteLyricsFile(filePath, lyrics); err != nil {
			log.WithCtx(ctx).Error(fmt.Sprintf("写入 .lrc 失败: %v", err))
		} else {
			log.WithCtx(ctx).Info("✅ 已保存 .lrc 文件（ffmpeg 不可用）")
		}
	}
	if len(coverData) > 0 {
		if err := metadata.WriteCoverFile(filePath, coverData); err != nil {
			log.WithCtx(ctx).Error(fmt.Sprintf("写入封面文件失败: %v", err))
		} else {
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已保存封面文件 (%d KB, ffmpeg 不可用)", len(coverData)/1024))
		}
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
// 格式: "歌手+歌曲名"，如 "许嵩+放飞"
func buildSearchKeyword(title, artist string) string {
	if title != "" && artist != "" {
		return fmt.Sprintf("%s+%s", artist, title)
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
func findBestMatch(songs []provider.SongInfo, title, artist string) provider.SongInfo {
	if len(songs) == 0 {
		return provider.SongInfo{}
	}

	bestScore := -1
	bestIdx := 0

	titleLower := strings.ToLower(title)
	artistLower := strings.ToLower(artist)

	for i, song := range songs {
		score := 0
		songTitle := strings.ToLower(song.Title)
		songArtist := strings.ToLower(song.Artist)

		// 标题完全匹配
		if songTitle == titleLower {
			score += 100
		} else if strings.Contains(songTitle, titleLower) || strings.Contains(titleLower, songTitle) {
			score += 50
		}

		// 歌手匹配
		if songArtist == artistLower {
			score += 80
		} else if strings.Contains(songArtist, artistLower) || strings.Contains(artistLower, songArtist) {
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

// newProvider 根据名称创建元数据提供者
func newProvider(name, apiBase string) (provider.Provider, error) {
	switch name {
	case "netease":
		return provider.NewNetEaseProvider(apiBase), nil
	default:
		return nil, fmt.Errorf("未知的提供者: %s，可选: netease", name)
	}
}
