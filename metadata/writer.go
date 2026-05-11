package metadata

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
)

// WriteLyricsToMP3 将歌词写入 MP3 文件的 USLT 帧
func WriteLyricsToMP3(filePath, lyrics string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("打开 MP3 文件失败: %w", err)
	}
	defer tag.Close()

	lyricsFrame := id3v2.UnsynchronisedLyricsFrame{
		Encoding:          id3v2.EncodingUTF8,
		Language:          "eng",
		ContentDescriptor: "",
		Lyrics:            lyrics,
	}
	tag.AddUnsynchronisedLyricsFrame(lyricsFrame)

	if err := tag.Save(); err != nil {
		return fmt.Errorf("保存歌词到 MP3 失败: %w", err)
	}

	return nil
}

// WriteCoverToMP3 将封面图片写入 MP3 文件的 APIC 帧
func WriteCoverToMP3(filePath string, coverData []byte, mimeType string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("打开 MP3 文件失败: %w", err)
	}
	defer tag.Close()

	tag.DeleteFrames("APIC")

	picFrame := id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF8,
		MimeType:    mimeType,
		PictureType: id3v2.PTFrontCover,
		Description: "Cover",
		Picture:     coverData,
	}
	tag.AddAttachedPicture(picFrame)

	if err := tag.Save(); err != nil {
		return fmt.Errorf("保存封面到 MP3 失败: %w", err)
	}

	return nil
}

// WriteMetadataToMP3 将基础元数据写入 MP3 文件
func WriteMetadataToMP3(filePath, title, artist, album string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("打开 MP3 文件失败: %w", err)
	}
	defer tag.Close()

	if title != "" {
		tag.SetTitle(title)
	}
	if artist != "" {
		tag.SetArtist(artist)
	}
	if album != "" {
		tag.SetAlbum(album)
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("保存元数据到 MP3 失败: %w", err)
	}

	return nil
}

// WriteAllToMP3 一次性写入所有 MP3 元数据
func WriteAllToMP3(filePath, title, artist, album, date, lyrics string, coverData []byte, mimeType string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("打开 MP3 文件失败: %w", err)
	}
	defer tag.Close()

	if title != "" {
		tag.SetTitle(title)
	}
	if artist != "" {
		tag.SetArtist(artist)
	}
	if album != "" {
		tag.SetAlbum(album)
	}
	if date != "" {
		tag.SetYear(date)
	}

	if lyrics != "" {
		lyricsFrame := id3v2.UnsynchronisedLyricsFrame{
			Encoding:          id3v2.EncodingUTF8,
			Language:          "eng",
			ContentDescriptor: "",
			Lyrics:            lyrics,
		}
		tag.AddUnsynchronisedLyricsFrame(lyricsFrame)
	}

	if len(coverData) > 0 {
		tag.DeleteFrames("APIC")
		picFrame := id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    mimeType,
			PictureType: id3v2.PTFrontCover,
			Description: "Cover",
			Picture:     coverData,
		}
		tag.AddAttachedPicture(picFrame)
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("保存元数据到 MP3 失败: %w", err)
	}

	return nil
}

// WriteLyricsWithFFmpeg 使用 ffmpeg 将歌词嵌入到音频文件中（支持 FLAC、M4A、OGG 等）
// 对于 FLAC 文件，歌词写入 LYRICS 标签；对于其他格式，尝试写入 COMMENT 或对应标签
func WriteLyricsWithFFmpeg(filePath, lyrics string) error {
	return writeMetadataWithFFmpeg(filePath, "", "", "", "", lyrics, nil, "")
}

// WriteCoverWithFFmpeg 使用 ffmpeg 将封面图片嵌入到音频文件中
func WriteCoverWithFFmpeg(filePath string, coverData []byte) error {
	return writeMetadataWithFFmpeg(filePath, "", "", "", "", "", coverData, "")
}

// WriteLyricsAndCoverWithFFmpeg 同时写入歌词和封面（避免多次重写文件）
func WriteLyricsAndCoverWithFFmpeg(filePath, lyrics string, coverData []byte, mimeType string) error {
	return writeMetadataWithFFmpeg(filePath, "", "", "", "", lyrics, coverData, mimeType)
}

// WriteAllWithFFmpeg 使用 ffmpeg 一次性写入所有元数据（title、artist、album、date、歌词、封面）
func WriteAllWithFFmpeg(filePath, title, artist, album, date, lyrics string, coverData []byte, mimeType string) error {
	return writeMetadataWithFFmpeg(filePath, title, artist, album, date, lyrics, coverData, mimeType)
}

// writeMetadataWithFFmpeg 内部函数：同时写入元数据（title、artist、album、date、歌词和/或封面）
func writeMetadataWithFFmpeg(filePath, title, artist, album, date, lyrics string, coverData []byte, mimeType string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	tmpOut := filePath + ".tmp" + ext

	var args []string

	// 如果有封面数据，先写入临时图片文件
	var tmpImg *os.File
	if len(coverData) > 0 {
		var err error
		tmpImg, err = os.CreateTemp("", "cover-*.jpg")
		if err != nil {
			return fmt.Errorf("创建临时封面文件失败: %w", err)
		}
		defer os.Remove(tmpImg.Name())

		if _, err := tmpImg.Write(coverData); err != nil {
			tmpImg.Close()
			return fmt.Errorf("写入临时封面文件失败: %w", err)
		}
		tmpImg.Close()
	}

	switch ext {
	case ".wav":
		// WAV 格式：只支持基础元数据（标题、歌手、专辑、日期），不支持嵌入歌词和封面
		args = append(args, "-y", "-i", filePath)
		if title != "" {
			args = append(args, "-metadata", fmt.Sprintf("title=%s", title))
		}
		if artist != "" {
			args = append(args, "-metadata", fmt.Sprintf("artist=%s", artist))
		}
		if album != "" {
			args = append(args, "-metadata", fmt.Sprintf("album=%s", album))
		}
		if date != "" {
			args = append(args, "-metadata", fmt.Sprintf("date=%s", date))
		}
		// WAV 不支持嵌入歌词和封面，跳过 lyrics 和 coverData
		args = append(args, "-c:a", "copy", tmpOut)

	case ".flac":
		// FLAC 格式：同时处理元数据和封面
		args = append(args, "-y", "-i", filePath)
		if tmpImg != nil {
			args = append(args, "-i", tmpImg.Name())
		}
		if title != "" {
			args = append(args, "-metadata", fmt.Sprintf("title=%s", title))
		}
		if artist != "" {
			args = append(args, "-metadata", fmt.Sprintf("artist=%s", artist))
		}
		if album != "" {
			args = append(args, "-metadata", fmt.Sprintf("album=%s", album))
		}
		if date != "" {
			args = append(args, "-metadata", fmt.Sprintf("date=%s", date))
		}
		if lyrics != "" {
			args = append(args, "-metadata", fmt.Sprintf("lyrics=%s", lyrics))
		}
		if tmpImg != nil {
			args = append(args,
				"-map", "0:a",
				"-map", "1:0",
				"-c:a", "copy",
				"-metadata:s:v", "title=Cover",
				"-metadata:s:v", "comment=Front Cover",
				"-disposition:v", "attached_pic",
			)
		} else {
			args = append(args, "-c:a", "copy", "-c:v", "copy")
		}
		args = append(args, tmpOut)

	case ".m4a", ".aac":
		// M4A/AAC 格式（使用 iTunes 风格标签名）
		// 注意：ffmpeg 对 M4A 使用 "lyrics" 而非 "©lyr"，ffmpeg 会自动映射为 ©lyr atom
		args = append(args, "-y", "-i", filePath)
		if tmpImg != nil {
			args = append(args, "-i", tmpImg.Name())
		}
		if title != "" {
			args = append(args, "-metadata", fmt.Sprintf("title=%s", title))
		}
		if artist != "" {
			args = append(args, "-metadata", fmt.Sprintf("artist=%s", artist))
		}
		if album != "" {
			args = append(args, "-metadata", fmt.Sprintf("album=%s", album))
		}
		if date != "" {
			args = append(args, "-metadata", fmt.Sprintf("date=%s", date))
		}
		if lyrics != "" {
			args = append(args, "-metadata", fmt.Sprintf("lyrics=%s", lyrics))
		}
		if tmpImg != nil {
			args = append(args,
				"-map", "0:a",
				"-map", "1:0",
				"-c:a", "copy",
				"-metadata:s:v", "title=Cover",
				"-metadata:s:v", "comment=Front Cover",
				"-disposition:v", "attached_pic",
			)
		} else {
			args = append(args, "-c:a", "copy", "-c:v", "copy")
		}
		args = append(args, tmpOut)

	default:
		// 通用方式
		args = append(args, "-y", "-i", filePath)
		if tmpImg != nil {
			args = append(args, "-i", tmpImg.Name())
		}
		if title != "" {
			args = append(args, "-metadata", fmt.Sprintf("title=%s", title))
		}
		if artist != "" {
			args = append(args, "-metadata", fmt.Sprintf("artist=%s", artist))
		}
		if album != "" {
			args = append(args, "-metadata", fmt.Sprintf("album=%s", album))
		}
		if date != "" {
			args = append(args, "-metadata", fmt.Sprintf("date=%s", date))
		}
		if lyrics != "" {
			args = append(args, "-metadata", fmt.Sprintf("lyrics=%s", lyrics))
		}
		if tmpImg != nil {
			args = append(args,
				"-map", "0:a",
				"-map", "1:0",
				"-c:a", "copy",
				"-metadata:s:v", "title=Cover",
				"-metadata:s:v", "comment=Front Cover",
				"-disposition:v", "attached_pic",
			)
		} else {
			args = append(args, "-c:a", "copy", "-c:v", "copy")
		}
		args = append(args, tmpOut)
	}

	cmd := exec.Command("ffmpeg", args...)
	// 屏蔽 ffmpeg 输出，避免干扰程序日志
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		os.Remove(tmpOut)
		return fmt.Errorf("ffmpeg 写入元数据失败: %w", err)
	}

	// 替换原文件
	if err := os.Rename(tmpOut, filePath); err != nil {
		os.Remove(tmpOut)
		return fmt.Errorf("替换原文件失败: %w", err)
	}

	return nil
}

// WriteLyricsFile 将歌词写入独立的 .lrc 文件
func WriteLyricsFile(musicPath, lyrics string) error {
	lrcPath := strings.TrimSuffix(musicPath, filepath.Ext(musicPath)) + ".lrc"
	if _, err := os.Stat(lrcPath); err == nil {
		return nil // 已存在，跳过
	}
	return os.WriteFile(lrcPath, []byte(lyrics), 0o644)
}

// WriteCoverFile 将封面图片保存为独立文件
func WriteCoverFile(musicPath string, coverData []byte) error {
	coverPath := strings.TrimSuffix(musicPath, filepath.Ext(musicPath)) + ".jpg"
	if _, err := os.Stat(coverPath); err == nil {
		return nil // 已存在，跳过
	}
	return os.WriteFile(coverPath, coverData, 0o644)
}

// IsMP3 检查文件是否为 MP3 格式
func IsMP3(filePath string) bool {
	return strings.ToLower(filepath.Ext(filePath)) == ".mp3"
}

// IsWAV 检查文件是否为 WAV 格式
func IsWAV(filePath string) bool {
	return strings.ToLower(filepath.Ext(filePath)) == ".wav"
}

// IsAPE 检查文件是否为 APE 格式
func IsAPE(filePath string) bool {
	return strings.ToLower(filepath.Ext(filePath)) == ".ape"
}

// RemoveMetadataFromMP3 从 MP3 文件中删除指定的元数据标签
// tags 参数指定要删除的标签名（如 "title", "artist", "album", "lyrics", "cover" 等）
func RemoveMetadataFromMP3(filePath string, tags []string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("打开 MP3 文件失败: %w", err)
	}
	defer tag.Close()

	for _, t := range tags {
		switch strings.ToLower(t) {
		case TagTitle, "tit2":
			tag.SetTitle("")
		case TagArtist, "tpe1":
			tag.SetArtist("")
		case TagAlbum, "talb":
			tag.SetAlbum("")
		case TagDate, "year", "tdrc":
			tag.SetYear("")
		case TagGenre, "tcon":
			tag.SetGenre("")
		case "lyrics", "uslt":
			tag.DeleteFrames("USLT")
		case "cover", "apic":
			tag.DeleteFrames("APIC")
		case TagComment, "comm":
			tag.DeleteFrames("COMM")
		case TagComposer, "tcom":
			tag.DeleteFrames("TCOM")
		case TagAlbumArtist, "tpe2":
			tag.DeleteFrames("TPE2")
		case TagCopyright, "tcop":
			tag.DeleteFrames("TCOP")
		case TagTrack, "trck":
			tag.DeleteFrames("TRCK")
		case TagDisc, "tpos":
			tag.DeleteFrames("TPOS")
		default:
			// 尝试删除自定义帧（大写帧 ID）
			tag.DeleteFrames(strings.ToUpper(t))
		}
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("保存 MP3 元数据失败: %w", err)
	}

	return nil
}

// RemoveMetadataWithFFmpeg 使用 ffmpeg 从音频文件中删除指定的元数据标签
// 通过设置 -metadata:key="" 来清除标签值
func RemoveMetadataWithFFmpeg(filePath string, tags []string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	tmpOut := filePath + ".tmp" + ext

	var args []string
	args = append(args, "-y", "-i", filePath)

	// 为每个要删除的标签添加 -metadata:key="" 参数
	for _, t := range tags {
		args = append(args, "-metadata", fmt.Sprintf("%s=", t))
	}

	// APE 格式不支持 ffmpeg muxer
	if IsAPE(filePath) {
		return fmt.Errorf("APE 格式不支持删除内嵌元数据")
	}

	// WAV 格式：只支持基础元数据删除
	if IsWAV(filePath) {
		args = append(args, "-c:a", "copy", tmpOut)
	} else {
		// 其他格式：同时清除封面（如果有 cover 标签）
		hasCover := false
		for _, t := range tags {
			if strings.ToLower(t) == "cover" {
				hasCover = true
				break
			}
		}
		if hasCover {
			args = append(args, "-map", "0:a", "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "copy", "-c:v", "copy")
		}
		args = append(args, tmpOut)
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		os.Remove(tmpOut)
		return fmt.Errorf("ffmpeg 删除元数据失败: %w", err)
	}

	if err := os.Rename(tmpOut, filePath); err != nil {
		os.Remove(tmpOut)
		return fmt.Errorf("替换原文件失败: %w", err)
	}

	return nil
}

// RemoveExternalFiles 删除与音乐文件关联的外部歌词和封面文件
// tags 参数指定要删除的类型："lyrics" 删除 .lrc 文件，"cover" 删除 .jpg 文件
func RemoveExternalFiles(filePath string, tags []string) []string {
	var removed []string

	for _, t := range tags {
		switch strings.ToLower(t) {
		case "lyrics":
			lrcPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".lrc"
			if _, err := os.Stat(lrcPath); err == nil {
				if err := os.Remove(lrcPath); err == nil {
					removed = append(removed, lrcPath)
				}
			}
		case "cover":
			coverPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".jpg"
			if _, err := os.Stat(coverPath); err == nil {
				if err := os.Remove(coverPath); err == nil {
					removed = append(removed, coverPath)
				}
			}
		}
	}

	return removed
}

// SetMetadataToMP3 设置 MP3 文件的自定义元数据标签
// tags 参数为 key=value 对的 map
func SetMetadataToMP3(filePath string, tags map[string]string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("打开 MP3 文件失败: %w", err)
	}
	defer tag.Close()

	for k, v := range tags {
		switch strings.ToLower(k) {
		case TagTitle, "tit2":
			tag.SetTitle(v)
		case TagArtist, "tpe1":
			tag.SetArtist(v)
		case TagAlbum, "talb":
			tag.SetAlbum(v)
		case TagDate, "year", "tdrc":
			tag.SetYear(v)
		case TagGenre, "tcon":
			tag.SetGenre(v)
		case TagComment, "comm":
			tag.DeleteFrames("COMM")
			if v != "" {
				commentFrame := id3v2.CommentFrame{
					Encoding:    id3v2.EncodingUTF8,
					Language:    "eng",
					Description: "",
					Text:        v,
				}
				tag.AddCommentFrame(commentFrame)
			}
		case TagComposer, "tcom":
			tag.DeleteFrames("TCOM")
			if v != "" {
				tag.AddTextFrame(tag.CommonID("Composer"), id3v2.EncodingUTF8, v)
			}
		case TagAlbumArtist, "tpe2":
			tag.DeleteFrames("TPE2")
			if v != "" {
				tag.AddTextFrame(tag.CommonID("Album artist"), id3v2.EncodingUTF8, v)
			}
		case TagCopyright, "tcop":
			tag.DeleteFrames("TCOP")
			if v != "" {
				tag.AddTextFrame(tag.CommonID("Copyright message"), id3v2.EncodingUTF8, v)
			}
		case TagTrack, "trck":
			tag.DeleteFrames("TRCK")
			if v != "" {
				tag.AddTextFrame(tag.CommonID("Track number/Position in set"), id3v2.EncodingUTF8, v)
			}
		case TagDisc, "tpos":
			tag.DeleteFrames("TPOS")
			if v != "" {
				tag.AddTextFrame(tag.CommonID("Part of a set"), id3v2.EncodingUTF8, v)
			}
		default:
			// 自定义帧：使用 TXXX (User defined text information) 帧
			if v != "" {
				frame := id3v2.UserDefinedTextFrame{
					Encoding:    id3v2.EncodingUTF8,
					Description: k,
					Value:       v,
				}
				tag.AddUserDefinedTextFrame(frame)
			}
		}
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("保存 MP3 元数据失败: %w", err)
	}

	return nil
}

// SetMetadataWithFFmpeg 使用 ffmpeg 设置自定义元数据标签
// tags 参数为 key=value 对的 map
func SetMetadataWithFFmpeg(filePath string, tags map[string]string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	tmpOut := filePath + ".tmp" + ext

	// APE 格式不支持 ffmpeg muxer
	if IsAPE(filePath) {
		return fmt.Errorf("APE 格式不支持写入内嵌元数据")
	}

	var args []string
	args = append(args, "-y", "-i", filePath)

	// 添加所有元数据标签
	for k, v := range tags {
		args = append(args, "-metadata", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, "-c:a", "copy", "-c:v", "copy", tmpOut)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		os.Remove(tmpOut)
		return fmt.Errorf("ffmpeg 设置元数据失败: %w", err)
	}

	if err := os.Rename(tmpOut, filePath); err != nil {
		os.Remove(tmpOut)
		return fmt.Errorf("替换原文件失败: %w", err)
	}

	return nil
}

// SupportsEmbedding 检查 ffmpeg 是否可用
func SupportsEmbedding() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}
