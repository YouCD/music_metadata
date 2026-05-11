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

// 标准标签 key 到 WAV INFO chunk ID 的反向映射
var stdTagToWavInfo = map[string]string{
	TagTitle:   wavInfoTitle,
	TagArtist:  wavInfoArtist,
	TagAlbum:   wavInfoAlbum,
	TagDate:    wavInfoDate,
	TagGenre:   wavInfoGenre,
	TagComment: wavInfoComment,
	TagTrack:   wavInfoTrack,
}

// WAV INFO chunk ID 到标准标签 key 的映射
var wavTagMapping = map[string]string{
	wavInfoTitle:   TagTitle,
	wavInfoArtist:  TagArtist,
	wavInfoAlbum:   TagAlbum,
	wavInfoDate:    TagDate,
	wavInfoGenre:   TagGenre,
	wavInfoComment: TagComment,
	wavInfoTrack:   TagTrack,
}

// ReadMusicFileFromWAV 使用纯 Go 读取 WAV 文件元数据，返回 MusicFile
func ReadMusicFileFromWAV(filePath string) (*MusicFile, error) {
	wavTags, err := ReadWAVMetadata(filePath)
	if err != nil {
		return nil, err
	}

	mf := &MusicFile{
		FilePath: filePath,
		Tags:     make(map[string]string),
	}

	// 将 WAV INFO chunk 标签映射到标准 Tags
	for wavKey, wavValue := range wavTags {
		if wavValue == "" {
			continue
		}
		if stdKey, ok := wavTagMapping[wavKey]; ok {
			mf.Tags[stdKey] = wavValue
		} else {
			// 未知标签，用原始 key（小写）存储
			mf.Tags[strings.ToLower(wavKey)] = wavValue
		}
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

// RemoveMetadataFromWAV 纯 Go 实现：从 WAV 文件中删除指定的元数据标签
// 通过直接操作 RIFF LIST/INFO chunk 来删除子 chunk，无需依赖 ffmpeg
func RemoveMetadataFromWAV(filePath string, tags []string) error {
	// 将标准标签名转换为 WAV INFO chunk ID
	var wavChunkIDs []string
	for _, t := range tags {
		if wavID, ok := stdTagToWavInfo[strings.ToLower(t)]; ok {
			wavChunkIDs = append(wavChunkIDs, wavID)
		} else {
			// 自定义标签，尝试用原始名称（大写）
			wavChunkIDs = append(wavChunkIDs, strings.ToUpper(t))
		}
	}

	// 读取整个文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 验证 RIFF 头
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return fmt.Errorf("不是有效的 WAV 文件")
	}

	// 查找 LIST/INFO chunk
	listOffset, listSize, infoStart, infoEnd, err := findLISTINFOChunk(data)
	if err != nil {
		// 没有 LIST/INFO chunk，无需删除
		return nil
	}

	// 收集需要删除的子 chunk 的偏移和大小
	type subChunkInfo struct {
		offset int64 // 子 chunk 在文件中的起始偏移（包含 8 字节头）
		size   int64 // 子 chunk 总占用大小（包含 8 字节头 + 数据 + 对齐填充）
	}
	var toRemove []subChunkInfo

	offset := infoStart
	for offset < infoEnd {
		if offset+8 > int64(len(data)) {
			break
		}
		subID := string(data[offset : offset+4])
		subSize := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))

		// 检查是否需要删除此子 chunk
		shouldRemove := false
		for _, id := range wavChunkIDs {
			if subID == id {
				shouldRemove = true
				break
			}
		}

		if shouldRemove {
			totalSize := int64(8) + subSize
			if subSize%2 != 0 {
				totalSize++ // RIFF chunks 按 word 对齐
			}
			toRemove = append(toRemove, subChunkInfo{offset: offset, size: totalSize})
		}

		// 移动到下一个子 chunk
		offset += 8 + subSize
		if subSize%2 != 0 {
			offset++
		}
	}

	if len(toRemove) == 0 {
		// 没有找到需要删除的子 chunk
		return nil
	}

	// 计算需要删除的总字节数
	var totalRemoved int64
	for _, rc := range toRemove {
		totalRemoved += rc.size
	}

	// 从后往前删除子 chunk（避免偏移变化）
	var newData []byte
	newData = append(newData, data[:listOffset]...) // LIST chunk 之前的数据

	// 重新构建 LIST/INFO chunk 内容
	// LIST 头（8字节）+ "INFO"（4字节）+ 保留的子 chunks
	listHeaderEnd := listOffset + 12                             // 跳过 "LIST" + size(4) + "INFO"
	newData = append(newData, data[listOffset:listHeaderEnd]...) // 保留 LIST 头和 INFO 类型

	// 复制未被删除的子 chunk 数据
	prevEnd := infoStart
	for _, rc := range toRemove {
		// 复制从上一个结束位置到当前删除位置之前的数据
		if rc.offset > prevEnd {
			newData = append(newData, data[prevEnd:rc.offset]...)
		}
		prevEnd = rc.offset + rc.size
	}
	// 复制最后一个删除位置之后到 INFO 结束的数据
	if prevEnd < infoEnd {
		newData = append(newData, data[prevEnd:infoEnd]...)
	}

	// 复制 LIST chunk 之后的数据
	afterListEnd := listOffset + 8 + listSize
	if int(afterListEnd) < len(data) {
		newData = append(newData, data[afterListEnd:]...)
	}

	// 更新 LIST chunk 大小
	newListSize := uint32(listSize - totalRemoved)
	binary.LittleEndian.PutUint32(newData[listOffset+4:listOffset+8], newListSize)

	// 更新 RIFF 头大小
	newRiffSize := uint32(len(newData) - 8)
	binary.LittleEndian.PutUint32(newData[4:8], newRiffSize)

	// 检查删除后 INFO chunk 是否还有子 chunk
	// 如果 INFO chunk 只剩 "INFO" 类型标识（4字节），则整个 LIST chunk 也应删除
	hasRemainingSubChunks := false
	checkOffset := infoStart
	for checkOffset < infoEnd-totalRemoved {
		if checkOffset+8 <= int64(len(newData)) {
			// 简单检查：如果还有数据在 INFO 区域，说明有子 chunk
			hasRemainingSubChunks = true
			break
		}
		checkOffset++
	}

	if !hasRemainingSubChunks {
		// INFO chunk 为空，删除整个 LIST chunk
		// 重新构建：LIST 之前 + LIST 之后
		finalData := make([]byte, 0, len(newData)-int(8+listSize-totalRemoved))
		finalData = append(finalData, newData[:listOffset]...)
		newAfterListEnd := listOffset + 8 + int64(newListSize)
		if int(newAfterListEnd) < len(newData) {
			finalData = append(finalData, newData[newAfterListEnd:]...)
		}
		// 更新 RIFF 头大小
		binary.LittleEndian.PutUint32(finalData[4:8], uint32(len(finalData)-8))
		newData = finalData
	}

	// 写回文件
	if err := os.WriteFile(filePath, newData, 0o644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// findLISTINFOChunk 在 WAV 文件数据中查找 LIST/INFO chunk
// 返回：LIST chunk 偏移、LIST chunk 大小、INFO 子 chunk 起始偏移、INFO 子 chunk 结束偏移
func findLISTINFOChunk(data []byte) (listOffset, listSize, infoStart, infoEnd int64, err error) {
	if len(data) < 12 {
		return 0, 0, 0, 0, fmt.Errorf("数据太短")
	}

	fileSize := int64(binary.LittleEndian.Uint32(data[4:8])) + 8

	offset := int64(12)
	for offset < fileSize {
		if offset+8 > int64(len(data)) {
			break
		}

		chunkID := string(data[offset : offset+4])
		chunkSize := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))

		if chunkID == "LIST" {
			if offset+12 > int64(len(data)) {
				break
			}
			listType := string(data[offset+8 : offset+12])
			if listType == "INFO" {
				listOffset = offset
				listSize = chunkSize
				infoStart = offset + 12 // 跳过 "LIST" + size + "INFO"
				infoEnd = offset + 8 + chunkSize
				return
			}
		}

		offset += 8 + chunkSize
		if chunkSize%2 != 0 {
			offset++
		}
	}

	return 0, 0, 0, 0, fmt.Errorf("未找到 LIST/INFO chunk")
}
