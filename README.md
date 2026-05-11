# 🎵 music_metadata

音乐元数据自动补全工具 — 自动抓取并嵌入歌词、封面、标题、歌手、专辑等信息到音频文件中。

## ✨ 功能特性

- 🔍 自动搜索匹配歌曲（网易云音乐数据源）
- 📝 嵌入歌词（支持翻译歌词合并）
- 🖼️ 嵌入封面图片
- 🏷️ 写入标题、歌手、专辑、日期等基础元数据
- 📊 查看音乐文件元数据信息
- ⚡ 并发处理，支持自定义并发数
- 🔄 智能跳过元信息完整的文件
- 📁 支持外部文件模式（.lrc / .jpg）

## 📦 支持的音频格式

| 格式 | 读取元数据 | 写入元数据 | 嵌入歌词 | 嵌入封面 |
|------|-----------|-----------|---------|---------|
| MP3  | ✅ id3v2  | ✅ id3v2  | ✅ USLT | ✅ APIC |
| FLAC | ✅        | ✅ ffmpeg | ✅      | ✅      |
| M4A  | ✅        | ✅ ffmpeg | ✅      | ✅      |
| WAV  | ✅ 纯Go解析 | ✅ ffmpeg | 📁 外部.lrc | 📁 外部.jpg |
| OGG  | ✅        | ✅ ffmpeg | ✅      | ✅      |
| AAC  | ✅        | ✅ ffmpeg | ✅      | ✅      |
| APE  | ✅ ffprobe | ❌ 不支持写入 | 📁 外部.lrc | 📁 外部.jpg |
| AIFF | ✅        | ✅ ffmpeg | ✅      | ✅      |

> **注意**：
> - 非 MP3 格式的元数据写入依赖 [ffmpeg](https://ffmpeg.org/)，WAV 文件的元数据读取使用纯 Go 实现（解析 RIFF LIST/INFO chunk）。
> - APE 格式由于 ffmpeg 不支持 APE muxer（仅支持解封装），元数据读取依赖 [ffprobe](https://ffmpeg.org/)，所有元数据（歌词、封面）只能保存为外部文件（`.lrc` / `.jpg`）。

## 🚀 安装

### go install（推荐）

```bash
go install github.com/YouCD/music_metadata@latest
```

### 从源码构建

```bash
git clone https://github.com/YouCD/music_metadata.git
cd music_metadata
make build
```

### 跨平台构建

```bash
make build-all
```

## 📖 使用方法

### scan — 扫描并补全元数据

递归扫描目录中的音乐文件，自动搜索匹配歌曲并嵌入元数据。

```bash
# 基本用法
music_metadata scan ./music

# 指定提供者
music_metadata scan ./music -p netease

# 预览模式（不修改文件）
music_metadata scan ./music --dry-run

# 强制更新已有元数据
music_metadata scan ./music --force

# 保存为外部文件（不嵌入音频文件）
music_metadata scan ./music --external

# 不获取歌词
music_metadata scan ./music --no-lyrics

# 不获取封面
music_metadata scan ./music --no-cover

# 设置并发数（默认 10）
music_metadata scan ./music -w 5
```

#### scan 选项

| 选项 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--provider` | `-p` | netease | 元数据提供者 |
| `--api` | | | 自定义 API 地址 |
| `--dry-run` | | false | 仅显示信息，不修改文件 |
| `--force` | `-f` | false | 强制更新已有元数据 |
| `--no-lyrics` | | false | 不获取歌词 |
| `--no-cover` | | false | 不获取封面 |
| `--external` | | false | 保存为外部 .lrc/.jpg 文件 |
| `--workers` | `-w` | 10 | 并发处理数 |

### info — 查看元数据信息

显示音乐文件的元数据，默认只显示元信息不完整的文件。

```bash
# 查看目录中元信息不完整的文件
music_metadata info ./music

# 查看单个文件
music_metadata info song.flac

# 显示所有文件（包括元信息完整的）
music_metadata info ./music --complete
music_metadata info ./music -c

# 显示详细信息（年份、流派、音轨）
music_metadata info ./music --all
music_metadata info ./music -a

# 组合使用
music_metadata info ./music -ac
```

#### info 选项

| 选项 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--all` | `-a` | false | 显示详细信息（年份、流派、音轨） |
| `--complete` | `-c` | false | 显示元信息完整的文件（默认只显示不完整的） |

### 全局选项

| 选项 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--provider` | `-p` | netease | 元数据提供者 |
| `--api` | | | 自定义 API 地址 |
| `--dry-run` | | false | 仅显示信息，不修改文件 |
| `--force` | `-f` | false | 强制更新已有元数据 |

## 🔧 工作原理

1. **文件名解析**：从文件名推断歌手和标题（支持 `歌手 - 标题`、`歌手-标题` 等格式）
2. **在线搜索**：通过网易云音乐 API 搜索匹配歌曲
3. **智能匹配**：根据标题和歌手进行评分，选择最佳匹配
4. **元数据写入**：
   - MP3：使用 [id3v2](https://github.com/bogem/id3v2) 库直接写入
   - FLAC/M4A/OGG/AAC/AIFF：通过 ffmpeg 嵌入元数据
   - WAV：基础元数据通过 ffmpeg 嵌入，歌词和封面保存为外部文件
   - APE：ffmpeg 不支持 APE muxer，所有元数据（歌词、封面）保存为外部文件

## 🏗️ 项目结构

```
.
├── main.go                  # 程序入口
├── cmd/
│   ├── root.go              # 根命令和全局选项
│   ├── scan.go              # scan 命令（扫描补全元数据）
│   └── info.go              # info 命令（查看元数据信息）
├── metadata/
│   ├── metadata.go          # 元数据读取和文件查找
│   ├── writer.go            # 元数据写入（id3v2 / ffmpeg）
│   └── wav.go               # WAV RIFF LIST/INFO 纯 Go 解析
├── provider/
│   ├── provider.go          # Provider 接口定义
│   └── netease.go           # 网易云音乐提供者实现
├── Makefile                 # 构建脚本
└── go.mod                   # Go 模块定义
```

## 📋 依赖

### Go 依赖

- [github.com/spf13/cobra](https://github.com/spf13/cobra) — CLI 框架
- [github.com/bogem/id3v2](https://github.com/bogem/id3v2) — MP3 ID3v2 标签读写
- [github.com/dhowden/tag](https://github.com/dhowden/tag) — 音频元数据读取（MP3/FLAC/M4A/OGG/AAC/AIFF）
- [github.com/pterm/pterm](https://github.com/pterm/pterm) — 进度条显示
- [github.com/olekukonko/tablewriter](https://github.com/olekukonko/tablewriter) — 表格渲染

### 外部工具依赖

- **[ffmpeg](https://ffmpeg.org/)**（必需）— 非 MP3 格式（FLAC、M4A、WAV、OGG、AAC、AIFF）的元数据写入和封面嵌入。MP3 格式使用纯 Go 库（id3v2）写入，不依赖 ffmpeg。
- **[ffprobe](https://ffmpeg.org/)**（随 ffmpeg 一起安装）— APE 格式的元数据读取。`dhowden/tag` 库不支持 APE 格式，需要通过 ffprobe 解析元数据。

> **安装 ffmpeg**：
> ```bash
> # macOS
> brew install ffmpeg
> 
> # Ubuntu / Debian
> sudo apt install ffmpeg
> 
> # Arch Linux
> sudo pacman -S ffmpeg
> ```
> 
> 安装 ffmpeg 后，`ffprobe` 会一同安装，无需单独安装。

## 📄 License

MIT License
