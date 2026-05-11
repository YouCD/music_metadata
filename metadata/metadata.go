package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// MusicFile 音乐文件元数据
type MusicFile struct {
	FilePath  string
	Title     string
	Artist    string
	Album     string
	Year      string
	Genre     string
	Comment   string
	Track     int
	HasLyrics bool
	HasCover  bool
	Format    tag.FileType
}

// ReadMusicFile 读取音乐文件的元数据
func ReadMusicFile(filePath string) (*MusicFile, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件 %s: %w", filePath, err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("无法读取文件 %s 的元数据: %w", filePath, err)
	}

	mf := &MusicFile{
		FilePath: filePath,
		Title:    m.Title(),
		Artist:   m.Artist(),
		Album:    m.Album(),
		Genre:    m.Genre(),
		Comment:  m.Comment(),
		Track:    func() int { track, _ := m.Track(); return track }(),
		Format:   m.FileType(),
	}

	if m.Year() != 0 {
		mf.Year = fmt.Sprintf("%d", m.Year())
	}

	// 检查是否有歌词
	if lrc, ok := m.Raw()["LYRICS"]; ok {
		if s, ok := lrc.(string); ok && s != "" {
			mf.HasLyrics = true
		}
	}
	if m.Lyrics() != "" {
		mf.HasLyrics = true
	}

	// 检查是否有封面图片
	if m.Picture() != nil {
		mf.HasCover = true
	}

	return mf, nil
}

// GetFormatExt 根据文件类型返回扩展名提示
func (mf *MusicFile) GetFormatExt() string {
	return strings.ToLower(filepath.Ext(mf.FilePath))
}

// String 打印音乐文件信息
func (mf *MusicFile) String() string {
	return fmt.Sprintf(
		"文件: %s\n标题: %s\n歌手: %s\n专辑: %s\n年份: %s\n格式: %s\n有歌词: %v\n有封面: %v",
		mf.FilePath, mf.Title, mf.Artist, mf.Album, mf.Year, mf.Format, mf.HasLyrics, mf.HasCover,
	)
}

// IsComplete 检查元信息是否完整（标题、歌手、专辑、歌词、封面都存在）
func (mf *MusicFile) IsComplete() bool {
	return mf.Title != "" && mf.Artist != "" && mf.Album != "" && mf.HasLyrics && mf.HasCover
}

// IsSupported 检查文件格式是否受支持
func IsSupported(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	supportedExts := map[string]bool{
		".mp3":  true,
		".flac": true,
		".m4a":  true,
		".ogg":  true,
		".opus": true,
		".wav":  true,
		".aac":  true,
		".wma":  true,
		".ape":  true,
		".aiff": true,
	}
	return supportedExts[ext]
}

// FindMusicFiles 在目录中递归查找所有支持的音乐文件
func FindMusicFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if IsSupported(path) {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("遍历目录 %s 失败: %w", dir, err)
	}

	return files, nil
}
