package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/YouCD/music_metadata/metadata"

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
		"  music_metadata remove song.flac --tag title,artist --dry-run\n",
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

	successCount := 0
	failedCount := 0

	for _, filePath := range files {
		if err := removeMetadataFromFile(filePath, tags, cmd.Context()); err != nil {
			log.WithCtx(cmd.Context()).Error(fmt.Sprintf("❌ %s: %v", filePath, err))
			failedCount++
		} else {
			successCount++
		}
	}

	log.WithCtx(cmd.Context()).Infof("📊 删除完成 - 总计: %d, 成功: %d, 失败: %d",
		len(files), successCount, failedCount)

	return nil
}

// removeMetadataFromFile 从单个文件中删除指定的元数据
func removeMetadataFromFile(filePath string, tags []string, ctx context.Context) error {
	// 区分内嵌标签和外部文件标签
	var embedTags []string
	var externalTags []string

	for _, t := range tags {
		switch strings.ToLower(t) {
		case "lyrics":
			// 歌词：同时删除内嵌和外部文件
			embedTags = append(embedTags, t)
			externalTags = append(externalTags, t)
		case "cover":
			// 封面：同时删除内嵌和外部文件
			embedTags = append(embedTags, t)
			externalTags = append(externalTags, t)
		default:
			embedTags = append(embedTags, t)
		}
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
		} else if metadata.IsAPE(filePath) {
			// APE 不支持删除内嵌元数据
			log.WithCtx(ctx).Warn("⚠️  APE 格式不支持删除内嵌元数据，仅删除外部文件")
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
