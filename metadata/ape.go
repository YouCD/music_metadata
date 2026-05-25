package metadata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

// APETagItem 表示 APE Tag 中的一个项目
type APETagItem struct {
	ValueLength uint32
	Flags       uint32
	Key         string
	Value       []byte
}

// RemoveMetadataFromAPE 从 APE 文件中删除指定的元数据标签
func RemoveMetadataFromAPE(filePath string, tags []string) error {
	// 读取整个文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 查找 APE Tag Footer (从文件末尾往前找)
	footerPos := bytes.LastIndex(data, []byte("APETAGEX"))
	if footerPos == -1 {
		// 没有 APE Tag，无需删除
		return nil
	}

	// 确保找到的是 Footer 而不是 Header
	// Footer 应该在文件的最后 32 字节内
	if footerPos < len(data)-32 {
		// 可能有多个 APETAGEX，找最后一个
		lastPos := footerPos
		for {
			nextPos := bytes.Index(data[lastPos+8:], []byte("APETAGEX"))
			if nextPos == -1 {
				break
			}
			lastPos = lastPos + 8 + nextPos
		}
		footerPos = lastPos
	}

	// 解析 Footer
	if footerPos+32 > len(data) {
		return fmt.Errorf("APE Tag Footer 不完整")
	}

	footer := data[footerPos : footerPos+32]

	// 检查魔数
	if !bytes.Equal(footer[0:8], []byte("APETAGEX")) {
		return fmt.Errorf("无效的 APE Tag Footer")
	}

	version := binary.LittleEndian.Uint32(footer[8:12])
	tagSize := binary.LittleEndian.Uint32(footer[12:16])
	itemCount := binary.LittleEndian.Uint32(footer[16:20])
	flags := binary.LittleEndian.Uint32(footer[20:24])

	// 验证版本
	if version != 2000 {
		return fmt.Errorf("不支持的 APE Tag 版本: %d", version)
	}

	// Footer 的 flags 应该是 0x80000000
	if flags != 0x80000000 {
		return fmt.Errorf("无效的 APE Tag Footer flags: 0x%08X", flags)
	}

	// 计算 Tag 数据的起始位置
	// tagSize 表示 Items + Footer 的大小（不包括 Header）
	tagItemsStart := footerPos - int(tagSize) + 32 // +32 是因为要从 Footer 前跳到 Items 开始
	var tagDataStart int

	// 检查是否有 Header
	headerPos := tagItemsStart - 32
	if headerPos >= 0 && bytes.Equal(data[headerPos:headerPos+8], []byte("APETAGEX")) {
		// 有 Header
		tagDataStart = headerPos
	} else {
		// 没有 Header，从 Items 开始
		tagDataStart = tagItemsStart
	}

	// 解析所有 Tag Items（从 Items 起始位置开始，跳过 Header）
	itemsStart := tagItemsStart
	items, err := parseAPETagItems(data[itemsStart:footerPos], int(itemCount))
	if err != nil {
		return fmt.Errorf("解析 APE Tag Items 失败: %w", err)
	}

	// 过滤掉要删除的标签
	var filteredItems []APETagItem
	deletedTags := make(map[string]bool)

	for _, item := range items {
		shouldDelete := false
		for _, tag := range tags {
			if equalFold(item.Key, tag) {
				shouldDelete = true
				deletedTags[tag] = true
				break
			}
		}

		if !shouldDelete {
			filteredItems = append(filteredItems, item)
		}
	}

	// 如果没有删除任何标签，直接返回
	if len(deletedTags) == 0 {
		return nil
	}

	// 重建 Tag 数据
	newTagData, err := buildAPETagData(filteredItems, version)
	if err != nil {
		return fmt.Errorf("构建新的 APE Tag 数据失败: %w", err)
	}

	// 构建新的文件内容
	var newFileData []byte

	// 添加音频数据部分（Tag 之前的所有内容）
	newFileData = append(newFileData, data[:tagDataStart]...)

	// 添加新的 Tag 数据
	newFileData = append(newFileData, newTagData...)

	// 写入新文件
	if err := os.WriteFile(filePath, newFileData, 0o644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// parseAPETagItems 解析 APE Tag Items
func parseAPETagItems(data []byte, itemCount int) ([]APETagItem, error) {
	var items []APETagItem
	offset := 0

	for i := 0; i < itemCount; i++ {
		if offset+8 > len(data) {
			return nil, fmt.Errorf("Item %d 数据不足", i+1)
		}

		valueLength := binary.LittleEndian.Uint32(data[offset : offset+4])
		itemFlags := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		offset += 8

		// 查找 Key（以 null 结尾）
		keyEnd := bytes.IndexByte(data[offset:], 0)
		if keyEnd == -1 {
			return nil, fmt.Errorf("Item %d 找不到 Key 结束符", i+1)
		}

		key := string(data[offset : offset+keyEnd])
		offset += keyEnd + 1

		// 读取 Value
		if offset+int(valueLength) > len(data) {
			return nil, fmt.Errorf("Item %d (%s) Value 超出范围", i+1, key)
		}

		value := make([]byte, valueLength)
		copy(value, data[offset:offset+int(valueLength)])
		offset += int(valueLength)

		items = append(items, APETagItem{
			ValueLength: valueLength,
			Flags:       itemFlags,
			Key:         key,
			Value:       value,
		})
	}

	return items, nil
}

// buildAPETagData 构建 APE Tag 数据（包括 Header、Items 和 Footer）
func buildAPETagData(items []APETagItem, version uint32) ([]byte, error) {
	var buf bytes.Buffer

	// 计算 Items 的总大小
	itemsSize := 0
	for _, item := range items {
		itemsSize += 4                     // ValueLength
		itemsSize += 4                     // Flags
		itemsSize += len(item.Key) + 1     // Key + null terminator
		itemsSize += int(item.ValueLength) // Value
	}

	// Tag 大小 = Items + Footer (32 bytes)
	tagSize := uint32(itemsSize + 32)

	// 写入 Header
	buf.Write([]byte("APETAGEX"))
	binary.Write(&buf, binary.LittleEndian, version)
	binary.Write(&buf, binary.LittleEndian, tagSize)
	binary.Write(&buf, binary.LittleEndian, uint32(len(items)))
	binary.Write(&buf, binary.LittleEndian, uint32(0xA0000000)) // Header flags
	buf.Write(make([]byte, 8))                                  // Reserved

	// 写入 Items
	for _, item := range items {
		binary.Write(&buf, binary.LittleEndian, item.ValueLength)
		binary.Write(&buf, binary.LittleEndian, item.Flags)
		buf.WriteString(item.Key)
		buf.WriteByte(0) // Null terminator
		buf.Write(item.Value)
	}

	// 写入 Footer
	buf.Write([]byte("APETAGEX"))
	binary.Write(&buf, binary.LittleEndian, version)
	binary.Write(&buf, binary.LittleEndian, tagSize)
	binary.Write(&buf, binary.LittleEndian, uint32(len(items)))
	binary.Write(&buf, binary.LittleEndian, uint32(0x80000000)) // Footer flags
	buf.Write(make([]byte, 8))                                  // Reserved

	return buf.Bytes(), nil
}

// equalFold 不区分大小写的字符串比较
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca := a[i]
		cb := b[i]
		// 转换为小写比较
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
