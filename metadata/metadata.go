package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// MusicFile 音乐文件元数据
type MusicFile struct {
	FilePath  string
	Tags      map[string]string // 所有标签（来自 ffprobe 或 dhowden/tag）
	HasLyrics bool
	HasCover  bool
	Format    tag.FileType
}

// 常用标签 key 常量
const (
	TagTitle         = "title"
	TagArtist        = "artist"
	TagAlbum         = "album"
	TagAlbumArtist   = "album_artist"
	TagComposer      = "composer"
	TagDate          = "date"
	TagGenre         = "genre"
	TagComment       = "comment"
	TagCopyright     = "copyright"
	TagTrack         = "track"
	TagDisc          = "disc"
	TagLyrics        = "lyrics"
	TagDuration      = "duration"
	TagBitRate       = "bit_rate"
	TagSampleRate    = "sample_rate"
	TagChannels      = "channels"
	TagBitsPerSample = "bits_per_sample"
)

// GetTag 获取标签值，支持多个候选 key（大小写兼容）
func (mf *MusicFile) GetTag(keys ...string) string {
	for _, key := range keys {
		// 精确匹配
		if v, ok := mf.Tags[key]; ok && v != "" {
			return v
		}
		// 大小写不敏感匹配
		lowerKey := strings.ToLower(key)
		for k, v := range mf.Tags {
			if strings.ToLower(k) == lowerKey && v != "" {
				return v
			}
		}
	}
	return ""
}

// GetTitle 获取标题
func (mf *MusicFile) GetTitle() string {
	return mf.GetTag(TagTitle, "TITLE", "Title")
}

// GetArtist 获取歌手
func (mf *MusicFile) GetArtist() string {
	return mf.GetTag(TagArtist, "ARTIST", "Artist")
}

// GetAlbum 获取专辑
func (mf *MusicFile) GetAlbum() string {
	return mf.GetTag(TagAlbum, "ALBUM", "Album")
}

// GetDate 获取日期
func (mf *MusicFile) GetDate() string {
	return mf.GetTag(TagDate, "DATE", "Date", "year", "YEAR", "Year")
}

// GetGenre 获取流派
func (mf *MusicFile) GetGenre() string {
	return mf.GetTag(TagGenre, "GENRE", "Genre")
}

// GetComment 获取注释
func (mf *MusicFile) GetComment() string {
	return mf.GetTag(TagComment, "COMMENT", "Comment")
}

// GetAlbumArtist 获取专辑艺术家
func (mf *MusicFile) GetAlbumArtist() string {
	return mf.GetTag(TagAlbumArtist, "ALBUM_ARTIST", "Album_Artist", "band")
}

// GetComposer 获取作曲
func (mf *MusicFile) GetComposer() string {
	return mf.GetTag(TagComposer, "COMPOSER", "Composer")
}

// GetCopyright 获取版权
func (mf *MusicFile) GetCopyright() string {
	return mf.GetTag(TagCopyright, "COPYRIGHT", "Copyright")
}

// GetTrack 获取音轨号
func (mf *MusicFile) GetTrack() string {
	return mf.GetTag(TagTrack, "TRACK", "Track", "track_number")
}

// GetDisc 获取碟片号
func (mf *MusicFile) GetDisc() string {
	return mf.GetTag(TagDisc, "DISC", "Disc", "disc_number")
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
		Tags:     make(map[string]string),
		Format:   m.FileType(),
	}

	// 将 dhowden/tag 读取的字段存入 Tags map
	if m.Title() != "" {
		mf.Tags[TagTitle] = m.Title()
	}
	if m.Artist() != "" {
		mf.Tags[TagArtist] = m.Artist()
	}
	if m.Album() != "" {
		mf.Tags[TagAlbum] = m.Album()
	}
	if m.Genre() != "" {
		mf.Tags[TagGenre] = m.Genre()
	}
	if m.Comment() != "" {
		mf.Tags[TagComment] = m.Comment()
	}
	if m.Year() != 0 {
		mf.Tags[TagDate] = fmt.Sprintf("%d", m.Year())
	}
	trackNum, trackTotal := m.Track()
	if trackNum != 0 {
		if trackTotal != 0 {
			mf.Tags[TagTrack] = fmt.Sprintf("%d/%d", trackNum, trackTotal)
		} else {
			mf.Tags[TagTrack] = fmt.Sprintf("%d", trackNum)
		}
	}
	discNum, discTotal := m.Disc()
	if discNum != 0 {
		if discTotal != 0 {
			mf.Tags[TagDisc] = fmt.Sprintf("%d/%d", discNum, discTotal)
		} else {
			mf.Tags[TagDisc] = fmt.Sprintf("%d", discNum)
		}
	}

	// 将 Raw() 中的所有标签也存入 Tags map
	for k, v := range m.Raw() {
		if s, ok := v.(string); ok && s != "" {
			// 避免覆盖已有的标准化 key
			lowerK := strings.ToLower(k)
			if _, exists := mf.Tags[lowerK]; !exists {
				mf.Tags[lowerK] = s
			}
		}
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
		mf.FilePath, mf.GetTitle(), mf.GetArtist(), mf.GetAlbum(), mf.GetDate(), mf.Format, mf.HasLyrics, mf.HasCover,
	)
}

// IsComplete 检查元信息是否完整（标题、歌手、专辑、歌词、封面都存在）
func (mf *MusicFile) IsComplete() bool {
	return mf.GetTitle() != "" && mf.GetArtist() != "" && mf.GetAlbum() != "" && mf.HasLyrics && mf.HasCover
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

// ffprobeFormat 表示 ffprobe -show_format 的输出结构
type ffprobeFormat struct {
	Tags     map[string]string `json:"tags"`
	Duration string            `json:"duration"`
	BitRate  string            `json:"bit_rate"`
}

type ffprobeStream struct {
	SampleRate    string `json:"sample_rate"`
	Channels      int    `json:"channels"`
	BitsPerSample int    `json:"bits_per_sample"`
	CodecType     string `json:"codec_type"`
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

// ReadMusicFileWithFFprobe 使用 ffprobe 读取音乐文件的元数据
// 当 dhowden/tag 无法读取某些格式（如 APE）时，回退到此方法
func ReadMusicFileWithFFprobe(filePath string) (*MusicFile, error) {
	if !SupportsEmbedding() {
		return nil, fmt.Errorf("ffprobe 不可用")
	}

	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	cmd.Stderr = nil
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe 执行失败: %w", err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}

	mf := &MusicFile{
		FilePath: filePath,
		Tags:     make(map[string]string),
	}

	// 读取 format 级别的标签
	tags := probe.Format.Tags
	if tags != nil {
		// 将所有标签存入 Tags map（key 统一为小写）
		for k, v := range tags {
			if v != "" {
				mf.Tags[strings.ToLower(k)] = v
			}
		}

		// 检查歌词
		if mf.GetTag("lyrics") != "" {
			mf.HasLyrics = true
		}
	}

	// 读取音频流信息
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			if stream.SampleRate != "" {
				mf.Tags[TagSampleRate] = stream.SampleRate
			}
			if stream.Channels > 0 {
				mf.Tags[TagChannels] = fmt.Sprintf("%d", stream.Channels)
			}
			if stream.BitsPerSample > 0 {
				mf.Tags[TagBitsPerSample] = fmt.Sprintf("%d", stream.BitsPerSample)
			}
			break // 只取第一个音频流
		}
	}

	// 读取 format 级别的时长和比特率
	if probe.Format.Duration != "" {
		mf.Tags[TagDuration] = probe.Format.Duration
	}
	if probe.Format.BitRate != "" {
		mf.Tags[TagBitRate] = probe.Format.BitRate
	}

	// 检查是否有外部歌词文件
	if !mf.HasLyrics {
		lrcPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".lrc"
		if _, err := os.Stat(lrcPath); err == nil {
			mf.HasLyrics = true
		}
	}

	// 检查是否有外部封面文件
	coverPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".jpg"
	if _, err := os.Stat(coverPath); err == nil {
		mf.HasCover = true
	}

	return mf, nil
}
