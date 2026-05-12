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

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/youcd/toolkit/log"
)

var (
	removeTags string
)

var removeCmd = &cobra.Command{
	Use:   "remove [文件或目录路径]",
	Short: "删除音乐文件中指定的元数据",
	Long: fmt.Sprintf("%s删除音乐文件中指定的元数据%s\n\n"+
		"从音乐文件中删除指定的元数据标签（如标题、歌手、歌词、封面等）。\n"+
		"支持删除内嵌元数据和外部关联文件（.lrc、.jpg）。\n\n"+
		"%s支持的标签名:%s\n"+
		"  title, artist, album, album_artist, date, genre, comment,\n"+
		"  composer, copyright, track, disc, lyrics, cover\n\n"+
		"%s示例:%s\n"+
		"  music_metadata remove song.mp3 --tag lyrics\n"+
		"  music_metadata remove song.mp3 --tag lyrics,cover\n"+
		"  music_metadata remove ./music --tag cover\n"+
		"  music_metadata remove song.flac --tag title,artist --dry-run\n"+
		"  music_metadata remove ./music --tag cover -w 5\n",
		ColorBold, ColorReset,
		ColorCyan, ColorReset,
		ColorCyan, ColorReset,
	),
	Args: cobra.MaximumNArgs(1),
	RunE: runRemove,
}

func init() {
	removeCmd.Flags().StringVarP(&removeTags, "tag", "t", "", "要删除的标签名，多个用逗号分隔（如 lyrics,cover）")
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// 检查 --tag 参数
	if removeTags == "" {
		return fmt.Errorf("请使用 --tag/-t 指定要删除的标签名（如 --tag lyrics,cover）")
	}

	// 解析标签列表
	tags := parseTagList(removeTags)
	if len(tags) == 0 {
		return fmt.Errorf("无效的标签名: %s", removeTags)
	}

	// 检查路径是否存在
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("路径不存在: %s", path)
	}

	// 获取文件列表
	var files []string
	if stat.IsDir() {
		files, err = metadata.FindMusicFiles(path)
		if err != nil {
			return fmt.Errorf("扫描目录失败: %w", err)
		}
		if len(files) == 0 {
			log.WithCtx(cmd.Context()).Warn("⚠️  目录中未找到支持的音乐文件")
			return nil
		}
	} else {
		if !metadata.IsSupported(path) {
			return fmt.Errorf("不支持的文件格式: %s", strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."))
		}
		files = []string{path}
	}

	log.WithCtx(cmd.Context()).Infof("🗑️  删除元数据 - 文件数: %d, 标签: %s", len(files), strings.Join(tags, ", "))

	// 统计（使用原子操作保证并发安全）
	var successCount int32
	var failedCount int32

	// 创建进度条，期间将日志级别设为 error，避免日志干扰进度条显示
	log.SetLogLevel("error")

	progressBar, err := pterm.DefaultProgressbar.WithTotal(len(files)).WithTitle("删除元数据").Start()
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

			relPath, _ := filepath.Rel(path, fp)

			if err := removeMetadataFromFile(fp, tags, cmd.Context()); err != nil {
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
	pterm.Info.Printfln("📊 删除完成 - 总计: %d, 成功: %d, 失败: %d",
		len(files), successCount, failedCount)

	return nil
}

// removeMetadataFromFile 从单个文件中删除指定的元数据
func removeMetadataFromFile(filePath string, tags []string, ctx context.Context) error {
	// 先读取文件元数据，检查要删除的标签是否存在
	mf, err := readMusicFileMetadata(filePath)
	if err != nil {
		return fmt.Errorf("读取元数据失败: %w", err)
	}

	// 检查每个标签是否存在，过滤出实际存在的标签
	var embedTags []string
	var externalTags []string
	var skippedTags []string

	for _, t := range tags {
		switch strings.ToLower(t) {
		case "lyrics":
			// 歌词：检查内嵌歌词和外部 .lrc 文件
			hasEmbed := mf != nil && mf.HasLyrics
			hasExternal := hasExternalLyricsFile(filePath)
			if !hasEmbed && !hasExternal {
				skippedTags = append(skippedTags, t)
				continue
			}
			if hasEmbed {
				embedTags = append(embedTags, t)
			}
			if hasExternal {
				externalTags = append(externalTags, t)
			}
		case "cover":
			// 封面：检查内嵌封面和外部 .jpg 文件
			hasEmbed := mf != nil && mf.HasCover
			hasExternal := hasExternalCoverFile(filePath)
			if !hasEmbed && !hasExternal {
				skippedTags = append(skippedTags, t)
				continue
			}
			if hasEmbed {
				embedTags = append(embedTags, t)
			}
			if hasExternal {
				externalTags = append(externalTags, t)
			}
		default:
			// 普通标签：检查 Tags map 中是否存在
			if mf == nil || !hasTagValue(mf, t) {
				skippedTags = append(skippedTags, t)
				continue
			}
			embedTags = append(embedTags, t)
		}
	}

	// 所有标签都不存在，跳过该文件
	if len(embedTags) == 0 && len(externalTags) == 0 {
		log.WithCtx(ctx).Info(fmt.Sprintf("⏭️  %s: 标签不存在，跳过 (%s)", filepath.Base(filePath), strings.Join(skippedTags, ", ")))
		return nil
	}

	if len(skippedTags) > 0 {
		log.WithCtx(ctx).Info(fmt.Sprintf("⏭️  %s: 标签不存在，跳过 (%s)", filepath.Base(filePath), strings.Join(skippedTags, ", ")))
	}

	// dry-run 模式下只显示预览信息
	if dryRun {
		var previewItems []string
		if len(externalTags) > 0 {
			previewItems = append(previewItems, fmt.Sprintf("外部文件(%s)", strings.Join(externalTags, ",")))
		}
		if len(embedTags) > 0 {
			previewItems = append(previewItems, fmt.Sprintf("内嵌标签(%s)", strings.Join(embedTags, ",")))
		}
		log.WithCtx(ctx).Info(fmt.Sprintf("🔍 预览模式，将删除 %s: %s", filePath, strings.Join(previewItems, ", ")))
		return nil
	}

	// 删除外部文件
	if len(externalTags) > 0 {
		removed := metadata.RemoveExternalFiles(filePath, externalTags)
		for _, f := range removed {
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ 已删除外部文件: %s", f))
		}
	}

	// 删除内嵌元数据
	if len(embedTags) > 0 {

		if metadata.IsMP3(filePath) {
			// MP3 使用 id3v2 库
			if err := metadata.RemoveMetadataFromMP3(filePath, embedTags); err != nil {
				return fmt.Errorf("删除 MP3 元数据失败: %w", err)
			}
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ %s: 已删除 MP3 标签: %s", filepath.Base(filePath), strings.Join(embedTags, ", ")))
		} else if metadata.IsWAV(filePath) {
			// WAV 使用纯 Go 删除 RIFF LIST/INFO chunk
			if err := metadata.RemoveMetadataFromWAV(filePath, embedTags); err != nil {
				return fmt.Errorf("删除 WAV 元数据失败: %w", err)
			}
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ %s: 已删除 WAV 标签: %s", filepath.Base(filePath), strings.Join(embedTags, ", ")))
		} else if metadata.IsAPE(filePath) {
			// APE 格式：使用自定义实现删除元数据
			if err := metadata.RemoveMetadataFromAPE(filePath, embedTags); err != nil {
				return fmt.Errorf("删除 APE 元数据失败: %w", err)
			}
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ %s: 已删除 APE 标签: %s", filepath.Base(filePath), strings.Join(embedTags, ", ")))
		} else if metadata.SupportsEmbedding() {
			// 其他格式使用 ffmpeg
			if err := metadata.RemoveMetadataWithFFmpeg(filePath, embedTags); err != nil {
				return fmt.Errorf("删除元数据失败: %w", err)
			}
			log.WithCtx(ctx).Info(fmt.Sprintf("✅ %s: 已删除标签 (via ffmpeg): %s", filepath.Base(filePath), strings.Join(embedTags, ", ")))
		} else {
			return fmt.Errorf("ffmpeg 不可用，无法删除内嵌元数据")
		}
	}

	return nil
}

// readMusicFileMetadata 读取音乐文件元数据（兼容各种格式）
func readMusicFileMetadata(filePath string) (*metadata.MusicFile, error) {
	var mf *metadata.MusicFile
	var err error

	// 尝试使用 dhowden/tag 读取
	mf, err = metadata.ReadMusicFile(filePath)
	if err != nil {
		// dhowden/tag 读取失败，尝试回退方案
		if metadata.IsWAV(filePath) {
			mf, err = metadata.ReadMusicFileFromWAV(filePath)
		}
	}

	// 尝试用 ffprobe 补充更多标签（ffprobe 能读取更多字段）
	if metadata.SupportsEmbedding() {
		if probeMf, probeErr := metadata.ReadMusicFileWithFFprobe(filePath); probeErr == nil && probeMf.Tags != nil {
			if mf == nil {
				mf = probeMf
				err = nil
			} else if mf.Tags == nil {
				mf.Tags = probeMf.Tags
			} else {
				for k, v := range probeMf.Tags {
					if _, exists := mf.Tags[k]; !exists && v != "" {
						mf.Tags[k] = v
					}
				}
			}
			if probeMf.HasLyrics {
				mf.HasLyrics = true
			}
			if probeMf.HasCover {
				mf.HasCover = true
			}
		}
	}

	if mf == nil {
		return nil, err
	}
	return mf, nil
}

// hasTagValue 检查 MusicFile 中指定标签是否有值
func hasTagValue(mf *metadata.MusicFile, tag string) bool {
	switch strings.ToLower(tag) {
	case metadata.TagTitle, "tit2":
		return mf.GetTitle() != ""
	case metadata.TagArtist, "tpe1":
		return mf.GetArtist() != ""
	case metadata.TagAlbum, "talb":
		return mf.GetAlbum() != ""
	case metadata.TagDate, "year", "tdrc":
		return mf.GetDate() != ""
	case metadata.TagGenre, "tcon":
		return mf.GetGenre() != ""
	case metadata.TagComment, "comm":
		return mf.GetComment() != ""
	case metadata.TagAlbumArtist, "tpe2":
		return mf.GetAlbumArtist() != ""
	case metadata.TagComposer, "tcom":
		return mf.GetComposer() != ""
	case metadata.TagCopyright, "tcop":
		return mf.GetCopyright() != ""
	case metadata.TagTrack, "trck":
		return mf.GetTrack() != ""
	case metadata.TagDisc, "tpos":
		return mf.GetDisc() != ""
	default:
		// 自定义标签：直接查 Tags map
		return mf.GetTag(tag) != ""
	}
}

// hasExternalLyricsFile 检查是否存在外部歌词文件
func hasExternalLyricsFile(filePath string) bool {
	lrcPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".lrc"
	_, err := os.Stat(lrcPath)
	return err == nil
}

// hasExternalCoverFile 检查是否存在外部封面文件
func hasExternalCoverFile(filePath string) bool {
	coverPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".jpg"
	_, err := os.Stat(coverPath)
	return err == nil
}

// parseTagList 解析逗号分隔的标签列表
func parseTagList(s string) []string {
	var result []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, strings.ToLower(t))
		}
	}
	return result
}
