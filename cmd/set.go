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

var setTags []string

var setCmd = &cobra.Command{
	Use:   "set [文件或目录路径]",
	Short: "设置音乐文件的自定义元数据",
	Long: fmt.Sprintf("%s设置音乐文件的自定义元数据%s\n\n"+
		"手动设置音乐文件的元数据标签，支持标准标签和自定义标签。\n"+
		"使用 --tag key=value 格式指定要设置的标签，可多次使用。\n\n"+
		"%s常用标签名:%s\n"+
		"  title, artist, album, album_artist, date, genre, comment,\n"+
		"  composer, copyright, track, disc, lyrics\n\n"+
		"  也支持任意自定义标签名。\n\n"+
		"%s示例:%s\n"+
		"  music_metadata set song.flac --tag title=新标题\n"+
		"  music_metadata set song.flac --tag artist=歌手 --tag album=专辑\n"+
		"  music_metadata set song.mp3 --tag genre=Pop --tag date=2024\n"+
		"  music_metadata set ./music --tag custom_field=自定义值\n"+
		"  music_metadata set song.flac --tag title=新标题 --dry-run\n",
		ColorBold, ColorReset,
		ColorCyan, ColorReset,
		ColorCyan, ColorReset,
	),
	Args: cobra.MaximumNArgs(1),
	RunE: runSet,
}

func init() {
	setCmd.Flags().StringArrayVar(&setTags, "tag", nil, "设置标签 key=value（可多次指定）")
	rootCmd.AddCommand(setCmd)
}

func runSet(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// 检查 --tag 参数
	if len(setTags) == 0 {
		return fmt.Errorf("请使用 --tag 指定要设置的标签（如 --tag title=新标题）")
	}

	// 解析标签 key=value 对
	tags, err := parseTagValues(setTags)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return fmt.Errorf("无效的标签格式")
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

	// 显示要设置的标签
	var tagDisplay []string
	for k, v := range tags {
		tagDisplay = append(tagDisplay, fmt.Sprintf("%s=%s", k, v))
	}
	log.WithCtx(cmd.Context()).Infof("✏️  设置元数据 - 文件数: %d, 标签: %s", len(files), strings.Join(tagDisplay, ", "))

	// 统计（使用原子操作保证并发安全）
	var successCount int32
	var failedCount int32

	// 创建进度条，期间将日志级别设为 error，避免日志干扰进度条显示
	log.SetLogLevel("error")

	progressBar, err := pterm.DefaultProgressbar.WithTotal(len(files)).WithTitle("设置元数据").Start()
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

			if err := setMetadataToFile(fp, tags, cmd.Context()); err != nil {
				atomic.AddInt32(&failedCount, 1)
				_ = relPath
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
	pterm.Info.Printfln("📊 设置完成 - 总计: %d, 成功: %d, 失败: %d",
		len(files), successCount, failedCount)

	return nil
}

// setMetadataToFile 设置单个文件的元数据
func setMetadataToFile(filePath string, tags map[string]string, ctx context.Context) error {
	// dry-run 模式下只显示预览信息
	if dryRun {
		var tagDisplay []string
		for k, v := range tags {
			tagDisplay = append(tagDisplay, fmt.Sprintf("%s=%s", k, v))
		}
		log.WithCtx(ctx).Info(fmt.Sprintf("🔍 预览模式，将设置 %s: %s", filePath, strings.Join(tagDisplay, ", ")))
		return nil
	}

	if metadata.IsMP3(filePath) {
		// MP3 使用 id3v2 库
		if err := metadata.SetMetadataToMP3(filePath, tags); err != nil {
			return fmt.Errorf("设置 MP3 元数据失败: %w", err)
		}
		log.WithCtx(ctx).Info(fmt.Sprintf("✅ %s: 已设置 MP3 标签", filepath.Base(filePath)))
	} else if metadata.IsAPE(filePath) {
		// APE 不支持写入内嵌元数据
		log.WithCtx(ctx).Warn(fmt.Sprintf("⚠️  %s: APE 格式不支持写入内嵌元数据", filepath.Base(filePath)))
		return fmt.Errorf("APE 格式不支持写入内嵌元数据")
	} else if metadata.SupportsEmbedding() {
		// 其他格式使用 ffmpeg
		if err := metadata.SetMetadataWithFFmpeg(filePath, tags); err != nil {
			return fmt.Errorf("设置元数据失败: %w", err)
		}
		log.WithCtx(ctx).Info(fmt.Sprintf("✅ %s: 已设置标签 (via ffmpeg)", filepath.Base(filePath)))
	} else {
		return fmt.Errorf("ffmpeg 不可用，无法设置内嵌元数据")
	}

	return nil
}

// parseTagValues 解析 --tag key=value 参数列表
func parseTagValues(tagArgs []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, arg := range tagArgs {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("无效的标签格式: %s（应为 key=value）", arg)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("标签名不能为空: %s", arg)
		}
		result[key] = value
	}
	return result, nil
}
