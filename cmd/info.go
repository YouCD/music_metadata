package cmd

import (
	"fmt"
	"music_metadata/metadata"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
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
			fmt.Printf("%s⚠️  目录中未找到支持的音乐文件%s\n", ColorYellow, ColorReset)
			return nil
		}
		fmt.Printf("找到 %d 个音乐文件:\n\n", len(files))
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
			fmt.Printf("[%d] %s%s%s\n", i+1, ColorRed, filePath, ColorReset)
			fmt.Printf("    %s❌ 读取失败: %v%s\n\n", ColorRed, err, ColorReset)
			continue
		}

		relPath, _ := filepath.Rel(".", filePath)
		fmt.Printf("[%d] %s%s%s\n", i+1, ColorCyan, relPath, ColorReset)
		fmt.Printf("    标题:    %s\n", displayValue(mf.Title))
		fmt.Printf("    歌手:    %s\n", displayValue(mf.Artist))
		fmt.Printf("    专辑:    %s\n", displayValue(mf.Album))

		if showAll || mf.Year != "" {
			fmt.Printf("    年份:    %s\n", displayValue(mf.Year))
		}
		if showAll || mf.Genre != "" {
			fmt.Printf("    流派:    %s\n", displayValue(mf.Genre))
		}
		if showAll || mf.Track != 0 {
			track := ""
			if mf.Track != 0 {
				track = fmt.Sprintf("%d", mf.Track)
			}
			fmt.Printf("    音轨:    %s\n", displayValue(track))
		}

		fmt.Printf("    格式:    %s\n", mf.Format)
		fmt.Printf("    歌词:    %s\n", boolIcon(mf.HasLyrics))
		fmt.Printf("    封面:    %s\n", boolIcon(mf.HasCover))
		fmt.Println()
	}

	return nil
}

func displayValue(s string) string {
	if s == "" {
		return fmt.Sprintf("%s(无)%s", ColorYellow, ColorReset)
	}
	return s
}
