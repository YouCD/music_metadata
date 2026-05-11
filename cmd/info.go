package cmd

import (
	"fmt"
	"music_metadata/metadata"
	"os"
	"path/filepath"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
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
	infoCmd.Flags().BoolVarP(&showAll, "all", "a", false, "显示详细信息（年份、流派、音轨）")
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// 将路径转为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("无法解析路径: %w", err)
	}

	// 检查路径是否存在
	stat, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("路径不存在: %s", absPath)
	}

	// 确定相对路径的基准目录
	var baseDir string
	var files []string
	if stat.IsDir() {
		baseDir = absPath
		// 扫描目录
		files, err = metadata.FindMusicFiles(absPath)
		if err != nil {
			return fmt.Errorf("扫描目录失败: %w", err)
		}
		if len(files) == 0 {
			log.WithCtx(cmd.Context()).Warn("⚠️  目录中未找到支持的音乐文件")
			return nil
		}
		log.WithCtx(cmd.Context()).Info(fmt.Sprintf("找到 %d 个音乐文件", len(files)))
	} else {
		baseDir = filepath.Dir(absPath)
		// 单个文件
		if !metadata.IsSupported(absPath) {
			return fmt.Errorf("不支持的文件格式: %s", filepath.Ext(absPath))
		}
		files = []string{absPath}
	}

	// 构建表头
	headers := []string{"#", "文件", "标题", "歌手", "专辑", "格式", "歌词", "封面"}
	if showAll {
		headers = append(headers, "年份", "流派", "音轨")
	}

	// 创建表格
	table := tablewriter.NewTable(os.Stdout,
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithHeaderAutoWrap(tw.WrapNone),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
	)
	table.Header(headers)

	// 填充数据
	for i, filePath := range files {
		relPath, _ := filepath.Rel(baseDir, filePath)
		ext := strings.ToLower(filepath.Ext(filePath))
		formatStr := strings.TrimPrefix(ext, ".")

		mf, err := metadata.ReadMusicFile(filePath)
		if err != nil {
			// 读取元数据失败时，仍然显示文件名和格式
			row := []string{
				fmt.Sprintf("%d", i+1),
				relPath,
				"-",
				"-",
				"-",
				formatStr,
				"❌",
				"❌",
			}
			if showAll {
				row = append(row, "-", "-", "-")
			}
			table.Append(row)
			continue
		}

		lyricsIcon := "❌"
		if mf.HasLyrics {
			lyricsIcon = "✅"
		}
		coverIcon := "❌"
		if mf.HasCover {
			coverIcon = "✅"
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			relPath,
			displayValue(mf.Title),
			displayValue(mf.Artist),
			displayValue(mf.Album),
			string(mf.Format),
			lyricsIcon,
			coverIcon,
		}

		if showAll {
			track := ""
			if mf.Track != 0 {
				track = fmt.Sprintf("%d", mf.Track)
			}
			row = append(row,
				displayValue(mf.Year),
				displayValue(mf.Genre),
				displayValue(track),
			)
		}

		table.Append(row)
	}

	// 渲染表格
	if err := table.Render(); err != nil {
		return fmt.Errorf("渲染表格失败: %w", err)
	}

	return nil
}

func displayValue(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
