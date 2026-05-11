package cmd

import (
	"fmt"
	"music_metadata/metadata"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/youcd/toolkit/log"
)

var infoCmd = &cobra.Command{
	Use:   "info [文件或目录路径]",
	Short: "查看音乐文件的元数据信息",
	Long: fmt.Sprintf("%s查看音乐文件的元数据信息%s\n\n"+
		"显示指定音乐文件或目录中所有音乐文件的元数据，\n"+
		"包括标题、歌手、专辑、歌词、封面等信息。\n\n"+
		"%s示例:%s\n"+
		"  music_metadata info song.flac\n"+
		"  music_metadata info ./music\n"+
		"  music_metadata info ./music --all\n",
		ColorBold, ColorReset,
		ColorCyan, ColorReset,
	),
	Args: cobra.MaximumNArgs(1),
	RunE: runInfo,
}

var showAll bool

func init() {
	infoCmd.Flags().BoolVarP(&showAll, "all", "a", false, "显示详细信息")
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// 检查路径是否存在
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("路径不存在: %s", path)
	}

	var files []string
	if stat.IsDir() {
		// 扫描目录
		files, err = metadata.FindMusicFiles(path)
		if err != nil {
			return fmt.Errorf("扫描目录失败: %w", err)
		}
		if len(files) == 0 {
			log.WithCtx(cmd.Context()).Warn("⚠️  目录中未找到支持的音乐文件")
			return nil
		}
		log.WithCtx(cmd.Context()).Info(fmt.Sprintf("找到 %d 个音乐文件", len(files)))
	} else {
		// 单个文件
		if !metadata.IsSupported(path) {
			return fmt.Errorf("不支持的文件格式: %s", filepath.Ext(path))
		}
		files = []string{path}
	}

	for i, filePath := range files {
		mf, err := metadata.ReadMusicFile(filePath)
		if err != nil {
			log.WithCtx(cmd.Context()).Error(fmt.Sprintf("[%d] %s - 读取失败: %v", i+1, filePath, err))
			continue
		}

		relPath, _ := filepath.Rel(".", filePath)
		log.WithCtx(cmd.Context()).Infof("[%d] %s - 标题: %s, 歌手: %s, 专辑: %s, 格式: %s, 有歌词: %v, 有封面: %v",
			i+1, relPath, mf.Title, mf.Artist, mf.Album, mf.Format, mf.HasLyrics, mf.HasCover)

		if showAll || mf.Year != "" {
			log.WithCtx(cmd.Context()).Debug(fmt.Sprintf("    年份: %s", displayValue(mf.Year)))
		}
		if showAll || mf.Genre != "" {
			log.WithCtx(cmd.Context()).Debug(fmt.Sprintf("    流派: %s", displayValue(mf.Genre)))
		}
		if showAll || mf.Track != 0 {
			track := ""
			if mf.Track != 0 {
				track = fmt.Sprintf("%d", mf.Track)
			}
			log.WithCtx(cmd.Context()).Debug(fmt.Sprintf("    音轨: %s", displayValue(track)))
		}
	}

	return nil
}

func displayValue(s string) string {
	if s == "" {
		return "(无)"
	}
	return s
}
