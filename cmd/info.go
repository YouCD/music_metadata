package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/YouCD/music_metadata/metadata"

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
		"包括标题、歌手、专辑、歌词、封面等信息。\n"+
		"默认只显示元信息不完整的文件，使用 --complete 显示所有。\n\n"+
		"%s示例:%s\n"+
		"  music_metadata info song.flac\n"+
		"  music_metadata info ./music\n"+
		"  music_metadata info ./music --all\n"+
		"  music_metadata info ./music --complete\n"+
		"  music_metadata info ./music -ac\n",
		ColorBold, ColorReset,
		ColorCyan, ColorReset,
	),
	Args: cobra.MaximumNArgs(1),
	RunE: runInfo,
}

var (
	showAll      bool
	showComplete bool
)

func init() {
	infoCmd.Flags().BoolVarP(&showAll, "all", "a", false, "显示详细信息（年份、流派、音轨）")
	infoCmd.Flags().BoolVarP(&showComplete, "complete", "c", false, "显示元信息完整的音乐文件（默认只显示不完整的）")
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

	// 扩展列定义：key 和对应的 getter 函数
	type extColumn struct {
		key    string
		getter func(*metadata.MusicFile) string
	}
	extColumns := []extColumn{
		{"date", (*metadata.MusicFile).GetDate},
		{"genre", (*metadata.MusicFile).GetGenre},
		{"track", (*metadata.MusicFile).GetTrack},
		{"comment", (*metadata.MusicFile).GetComment},
		{"composer", (*metadata.MusicFile).GetComposer},
		{"album_artist", (*metadata.MusicFile).GetAlbumArtist},
		{"copyright", (*metadata.MusicFile).GetCopyright},
	}

	// 第一遍：收集所有文件数据
	type fileInfo struct {
		relPath string
		mf      *metadata.MusicFile
		format  string
		readErr bool
	}
	var infos []fileInfo

	for _, filePath := range files {
		relPath, _ := filepath.Rel(baseDir, filePath)
		ext := strings.ToLower(filepath.Ext(filePath))
		formatStr := strings.TrimPrefix(ext, ".")

		mf, err := metadata.ReadMusicFile(filePath)
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

		fi := fileInfo{
			relPath: relPath,
			mf:      mf,
			format:  formatStr,
			readErr: err != nil && mf == nil,
		}
		infos = append(infos, fi)
	}

	// 判断哪些扩展列有值（至少一个文件有非空值）
	activeExtCols := make([]extColumn, 0, len(extColumns))
	if showAll {
		for _, col := range extColumns {
			hasValue := false
			for _, fi := range infos {
				if fi.mf != nil && col.getter(fi.mf) != "" {
					hasValue = true
					break
				}
			}
			if hasValue {
				activeExtCols = append(activeExtCols, col)
			}
		}
	}

	// 构建表头
	headers := []string{"#", "文件", "title", "artist", "album", "格式", "歌词", "封面"}
	for _, col := range activeExtCols {
		headers = append(headers, col.key)
	}

	// 创建表格
	table := tablewriter.NewTable(os.Stdout,
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithHeaderAutoWrap(tw.WrapNone),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
	)
	table.Header(headers)

	// 第二遍：填充数据
	rowNum := 0
	for _, fi := range infos {
		if fi.readErr {
			// 所有方式都失败，视为不完整，默认显示
			rowNum++
			row := []string{
				fmt.Sprintf("%d", rowNum),
				fi.relPath,
				"-",
				"-",
				"-",
				fi.format,
				"❌",
				"❌",
			}
			for range activeExtCols {
				row = append(row, "-")
			}
			table.Append(row)
			continue
		}

		// 默认只显示元信息不完整的文件，使用 --complete/-c 显示所有
		if !showComplete && fi.mf.IsComplete() {
			continue
		}

		rowNum++

		lyricsIcon := "❌"
		if fi.mf.HasLyrics {
			lyricsIcon = "✅"
		}
		coverIcon := "❌"
		if fi.mf.HasCover {
			coverIcon = "✅"
		}

		// 格式显示：优先使用 dhowden/tag 识别的格式，否则用文件扩展名，统一小写
		formatDisplay := strings.ToLower(string(fi.mf.Format))
		if formatDisplay == "" {
			formatDisplay = fi.format
		}

		row := []string{
			fmt.Sprintf("%d", rowNum),
			fi.relPath,
			displayValue(fi.mf.GetTitle()),
			displayValue(fi.mf.GetArtist()),
			displayValue(fi.mf.GetAlbum()),
			formatDisplay,
			lyricsIcon,
			coverIcon,
		}

		for _, col := range activeExtCols {
			row = append(row, displayValue(col.getter(fi.mf)))
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
