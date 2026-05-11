package metadata

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WAV INFO chunk 标签 ID 到字段的映射
// 参考: https://en.wikipedia.org/wiki/RIFF_(file_format)#INFO_chunk
const (
	wavInfoTitle   = "INAM" // Title
	wavInfoArtist  = "IART" // Artist
	wavInfoAlbum   = "IPRD" // Product (Album)
	wavInfoDate    = "ICRD" // Creation date
	wavInfoGenre   = "IGNR" // Genre
	wavInfoTrack   = "ITRK" // Track number
	wavInfoComment = "ICMT" // Comment
)

// ReadWAVMetadata 纯 Go 读取 WAV 文件的 RIFF LIST/INFO 元数据
// WAV 文件的元数据存储在 RIFF > LIST > INFO chunk 中
func ReadWAVMetadata(filePath string) (map[string]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件: %w", err)
	}
	defer f.Close()

	// 读取 RIFF 头
	var riffHeader [12]byte
	if _, err := f.Read(riffHeader[:]); err != nil {
		return nil, fmt.Errorf("读取 RIFF 头失败: %w", err)
	}

	// 验证 RIFF 标识
	if string(riffHeader[:4]) != "RIFF" {
		return nil, fmt.Errorf("不是有效的 RIFF 文件")
	}

	// 验证 WAVE 格式
	if string(riffHeader[8:12]) != "WAVE" {
		return nil, fmt.Errorf("不是 WAV 文件")
	}

	// 获取文件大小以限制扫描范围
	fileSize := int64(binary.LittleEndian.Uint32(riffHeader[4:8])) + 8

	tags := make(map[string]string)

	// 遍历 chunks，寻找 LIST chunk
	offset := int64(12)
	for offset < fileSize {
		// 读取 chunk 头 (4字节 ID + 4字节大小)
		var chunkHeader [8]byte
		if _, err := f.ReadAt(chunkHeader[:], offset); err != nil {
			break
		}

		chunkID := string(chunkHeader[:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))

		if chunkID == "LIST" {
			// 读取 LIST 类型
			var listType [4]byte
			if _, err := f.ReadAt(listType[:], offset+8); err != nil {
				break
			}

			if string(listType[:]) == "INFO" {
				// 解析 INFO 子 chunks
				parseINFOChunks(f, offset+12, offset+8+chunkSize, tags)
			}
		}

		// 移动到下一个 chunk（chunk 大小按 2 字节对齐）
		offset += 8 + chunkSize
		if chunkSize%2 != 0 {
			offset++ // RIFF chunks 按 word 对齐
		}
	}

	return tags, nil
}

// parseINFOChunks 解析 LIST/INFO chunk 中的子 chunks
func parseINFOChunks(f *os.File, start, end int64, tags map[string]string) {
	offset := start
	for offset < end {
		var subChunkHeader [8]byte
		if _, err := f.ReadAt(subChunkHeader[:], offset); err != nil {
			break
		}

		subID := string(subChunkHeader[:4])
		subSize := int64(binary.LittleEndian.Uint32(subChunkHeader[4:8]))

		if subSize > 0 && subSize < 4096 { // 合理的大小限制
			data := make([]byte, subSize)
			if _, err := f.ReadAt(data, offset+8); err != nil {
				break
			}

			// INFO chunk 的值是 null-terminated 字符串，去除尾部的 \x00
			value := strings.TrimRight(string(data), "\x00")
			tags[subID] = value
		}

		// 移动到下一个子 chunk
		offset += 8 + subSize
		if subSize%2 != 0 {
			offset++ // RIFF chunks 按 word 对齐
		}
	}
}

// ReadMusicFileFromWAV 使用纯 Go 读取 WAV 文件元数据，返回 MusicFile
func ReadMusicFileFromWAV(filePath string) (*MusicFile, error) {
	tags, err := ReadWAVMetadata(filePath)
	if err != nil {
		return nil, err
	}

	mf := &MusicFile{
		FilePath: filePath,
	}

	if v, ok := tags[wavInfoTitle]; ok {
		mf.Title = v
	}
	if v, ok := tags[wavInfoArtist]; ok {
		mf.Artist = v
	}
	if v, ok := tags[wavInfoAlbum]; ok {
		mf.Album = v
	}
	if v, ok := tags[wavInfoDate]; ok {
		mf.Year = v
	}
	if v, ok := tags[wavInfoGenre]; ok {
		mf.Genre = v
	}

	// 检查是否有外部歌词文件
	lrcPath := strings.TrimSuffix(filePath, strings.ToLower(filepath.Ext(filePath))) + ".lrc"
	if _, err := os.Stat(lrcPath); err == nil {
		mf.HasLyrics = true
	}

	// 检查是否有外部封面文件
	coverPath := strings.TrimSuffix(filePath, strings.ToLower(filepath.Ext(filePath))) + ".jpg"
	if _, err := os.Stat(coverPath); err == nil {
		mf.HasCover = true
	}

	return mf, nil
}
