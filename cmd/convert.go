package cmd

import (
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

var convertFormat string

var convertCmd = &cobra.Command{
	Use:   "convert [文件或目录路径]",
	Short: "转换音乐文件格式并保留元数据",
	Long: fmt.Sprintf("%s转换音乐文件格式并保留元数据%s\n\n"+
		"使用 ffmpeg 将音乐文件转换为指定格式，同时通过 -map_metadata 0 保留原始元数据。\n"+
		"支持 MP3、FLAC、M4A、OGG、WAV、APE、AIFF 等格式之间的相互转换。\n\n"+
		"%s注意:%s APE 格式不支持写入内嵌元数据，因此无法作为目标格式。\n\n"+
		"%s示例:%s\n"+
		"  music_metadata convert song.wav --format flac\n"+
		"  music_metadata convert song.flac --format mp3\n"+
		"  music_metadata convert ./music --format flac\n"+
		"  music_metadata convert ./music --format flac --dry-run\n"+
		"  music_metadata convert ./music --format flac -w 5\n",
		ColorBold, ColorReset,
		ColorYellow, ColorReset,
		ColorCyan, ColorReset,
	),
	Args: cobra.MaximumNArgs(1),
	RunE: runConvert,
}

func init() {
	convertCmd.Flags().StringVarP(&convertFormat, "format", "F", "flac", "目标格式（如 flac, mp3, m4a, ogg, wav, aiff）")
	rootCmd.AddCommand(convertCmd)
}

func runConvert(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// 验证目标格式
	convertFormat = strings.ToLower(convertFormat)
	if !isValidTargetFormat(convertFormat) {
		return fmt.Errorf("不支持的目标格式: %s（支持: flac, mp3, m4a, ogg, wav, aiff）", convertFormat)
	}

	// 检查 ffmpeg 是否可用
	if !metadata.SupportsEmbedding() {
		return fmt.Errorf("ffmpeg 未安装或不可用，转换功能需要 ffmpeg")
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

	log.WithCtx(cmd.Context()).Infof("🔄 格式转换 - 文件数: %d, 目标格式: %s", len(files), convertFormat)

	// 统计
	var successCount int32
	var failedCount int32
	var skippedCount int32

	// 创建进度条
	log.SetLogLevel("error")

	progressBar, err := pterm.DefaultProgressbar.WithTotal(len(files)).WithTitle("格式转换").Start()
	if err != nil {
		log.SetLogLevel("info")
		log.WithCtx(cmd.Context()).Error(fmt.Sprintf("创建进度条失败: %v", err))
	}

	// 使用 semaphore 控制并发数
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, filePath := range files {
		wg.Add(1)
		sem <- struct{}{}

		go func(fp string) {
			defer wg.Done()
			defer func() { <-sem }()

			relPath, _ := filepath.Rel(path, fp)

			// 检查是否已经是目标格式
			ext := strings.ToLower(filepath.Ext(fp))
			targetExt := "." + convertFormat
			if ext == targetExt {
				atomic.AddInt32(&skippedCount, 1)
				if progressBar != nil {
					progressBar.UpdateTitle(relPath + " (跳过)")
					progressBar.Increment()
				}
				return
			}

			if err := convertFile(fp, convertFormat, cmd); err != nil {
				atomic.AddInt32(&failedCount, 1)
				log.SetLogLevel("info")
				log.WithCtx(cmd.Context()).Error(fmt.Sprintf("❌ %s: 转换失败: %v", filepath.Base(fp), err))
				log.SetLogLevel("error")
			} else {
				atomic.AddInt32(&successCount, 1)
			}

			if progressBar != nil {
				progressBar.UpdateTitle(relPath)
				progressBar.Increment()
			}
		}(filePath)
	}

	wg.Wait()

	// 恢复日志级别
	log.SetLogLevel("info")

	// 打印汇总
	pterm.Info.Printfln("📊 转换完成 - 总计: %d, 成功: %d, 失败: %d, 跳过: %d",
		len(files), successCount, failedCount, skippedCount)

	return nil
}

// convertFile 转换单个文件格式
func convertFile(filePath string, targetFormat string, cmd *cobra.Command) error {
	// 构建目标文件路径
	ext := filepath.Ext(filePath)
	outputPath := strings.TrimSuffix(filePath, ext) + "." + targetFormat

	// dry-run 模式下只显示预览信息
	if dryRun {
		log.SetLogLevel("info")
		log.WithCtx(cmd.Context()).Info(fmt.Sprintf("🔍 预览模式，将转换 %s -> %s", filePath, outputPath))
		log.SetLogLevel("error")
		return nil
	}

	// 检查目标文件是否已存在
	if _, err := os.Stat(outputPath); err == nil {
		if !forceUpdate {
			log.SetLogLevel("info")
			log.WithCtx(cmd.Context()).Warn(fmt.Sprintf("⚠️  %s: 目标文件已存在，跳过（使用 --force 覆盖）", filepath.Base(outputPath)))
			log.SetLogLevel("error")
			return fmt.Errorf("目标文件已存在: %s", outputPath)
		}
	}

	// 调用 ffmpeg 进行转换，使用 -map_metadata 0 保留元数据
	if err := metadata.ConvertWithFFmpeg(filePath, outputPath); err != nil {
		return fmt.Errorf("ffmpeg 转换失败: %w", err)
	}

	log.SetLogLevel("error")
	log.WithCtx(cmd.Context()).Info(fmt.Sprintf("✅ %s -> %s: 转换成功", filepath.Base(filePath), filepath.Base(outputPath)))
	log.SetLogLevel("error")

	return nil
}

// isValidTargetFormat 检查目标格式是否有效
func isValidTargetFormat(format string) bool {
	validFormats := map[string]bool{
		"flac": true,
		"mp3":  true,
		"m4a":  true,
		"ogg":  true,
		"opus": true,
		"wav":  true,
		"aiff": true,
		"aac":  true,
		"wma":  true,
	}
	return validFormats[format]
}
