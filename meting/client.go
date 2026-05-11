package meting

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://api.i-meto.com/meting/api"
	defaultServer    = "netease"
	defaultSecretKey = "token"
)

// Client Meting API 客户端
type Client struct {
	BaseURL    string
	Server     string
	SecretKey  string
	HTTPClient *http.Client
}

// SongInfo 歌曲信息
type SongInfo struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	URL    string `json:"url"`
	Pic    string `json:"pic"`
	Lrc    string `json:"lrc"`
	// 用于后续请求的 ID，从 URL 中解析
	SongID string `json:"-"`
}

// NewClient 创建新的 Meting API 客户端
func NewClient(baseURL, server, secretKey string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if server == "" {
		server = defaultServer
	}
	if secretKey == "" {
		secretKey = defaultSecretKey
	}
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Server:    server,
		SecretKey: secretKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			// 不自动跟随重定向，因为 pic 和 url 接口返回 302
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// GenerateToken 生成 HMAC-SHA1 token
// token = HMAC-SHA1(secretKey, server + type + id)
func (c *Client) GenerateToken(typ, id string) string {
	message := fmt.Sprintf("%s%s%s", c.Server, typ, id)
	mac := hmac.New(sha1.New, []byte(c.SecretKey))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// buildURL 构建 API 请求 URL
func (c *Client) buildURL(typ, id, needAuth string) string {
	params := url.Values{}
	params.Set("server", c.Server)
	params.Set("type", typ)
	params.Set("id", id)
	if needAuth != "" {
		params.Set("auth", needAuth)
	}
	return fmt.Sprintf("%s?%s", c.BaseURL, params.Encode())
}

// Search 搜索歌曲
func (c *Client) Search(keyword string) ([]SongInfo, error) {
	apiURL := c.buildURL("search", keyword, "")

	resp, err := c.HTTPClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 读取错误响应体以便调试
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("搜索返回状态码: %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应失败: %w", err)
	}

	var songs []SongInfo
	if err := json.Unmarshal(body, &songs); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w, body: %s", err, string(body))
	}

	// 从 URL 中解析 song ID
	for i := range songs {
		songs[i].SongID = extractIDFromURL(songs[i].URL)
	}

	return songs, nil
}

// GetSongDetail 获取歌曲详情
func (c *Client) GetSongDetail(id string) ([]SongInfo, error) {
	apiURL := c.buildURL("song", id, "")
	resp, err := c.HTTPClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("获取歌曲详情失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取歌曲详情返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取歌曲详情响应失败: %w", err)
	}

	var songs []SongInfo
	if err := json.Unmarshal(body, &songs); err != nil {
		return nil, fmt.Errorf("解析歌曲详情失败: %w", err)
	}

	for i := range songs {
		songs[i].SongID = extractIDFromURL(songs[i].URL)
	}

	return songs, nil
}

// GetLyricsFromURL 从完整的 URL 获取歌词（URL 已包含 auth token）
func (c *Client) GetLyricsFromURL(lrcURL string) (string, error) {
	resp, err := c.HTTPClient.Get(lrcURL)
	if err != nil {
		return "", fmt.Errorf("获取歌词请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 检查是否是认证失败
		if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("获取歌词失败: API 认证失败 (状态码 401)，请检查 API 服务器是否可用")
		}
		return "", fmt.Errorf("获取歌词返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取歌词响应失败: %w", err)
	}

	return string(body), nil
}

// DownloadCoverFromURL 从完整的 URL 下载封面图片（URL 已包含 auth token）
func (c *Client) DownloadCoverFromURL(picURL string) ([]byte, string, error) {
	// 首先获取重定向后的真实 URL
	resp, err := c.HTTPClient.Get(picURL)
	if err != nil {
		return nil, "", fmt.Errorf("获取封面请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理重定向
	var finalURL string
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		finalURL = resp.Header.Get("Location")
		if finalURL == "" {
			return nil, "", fmt.Errorf("获取封面重定向失败：未找到 Location 头")
		}
	} else if resp.StatusCode == http.StatusOK {
		// 如果没有重定向，直接使用当前响应
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("读取封面图片数据失败: %w", err)
		}
		contentType := resp.Header.Get("Content-Type")
		return data, contentType, nil
	} else {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, "", fmt.Errorf("获取封面失败: API 认证失败 (状态码 401)，请检查 API 服务器是否可用")
		}
		return nil, "", fmt.Errorf("获取封面返回状态码: %d", resp.StatusCode)
	}

	// 下载重定向后的图片
	imgResp, err := http.Get(finalURL)
	if err != nil {
		return nil, "", fmt.Errorf("下载封面图片失败: %w", err)
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("下载封面图片返回状态码: %d", imgResp.StatusCode)
	}

	data, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取封面图片数据失败: %w", err)
	}

	contentType := imgResp.Header.Get("Content-Type")
	return data, contentType, nil
}

// GetLyrics 获取歌词（LRC 格式）
func (c *Client) GetLyrics(id string) (string, error) {
	token := c.GenerateToken("lrc", id)
	apiURL := c.buildURL("lrc", id, token)

	resp, err := c.HTTPClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("获取歌词请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 检查是否是认证失败
		if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("获取歌词失败: API 认证失败 (状态码 401)，请检查 API 服务器是否可用或需要配置自定义 token")
		}
		return "", fmt.Errorf("获取歌词返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取歌词响应失败: %w", err)
	}

	return string(body), nil
}

// GetCoverURL 获取封面图片 URL（返回 302 重定向的 Location）
func (c *Client) GetCoverURL(id string) (string, error) {
	token := c.GenerateToken("pic", id)
	apiURL := c.buildURL("pic", id, token)

	// 不跟随重定向，获取 Location 头
	resp, err := c.HTTPClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("获取封面请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		location := resp.Header.Get("Location")
		if location != "" {
			return location, nil
		}
	}

	// 如果没有重定向，尝试读取 body
	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	}

	// 检查是否是认证失败
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("获取封面失败: API 认证失败 (状态码 401)，请检查 API 服务器是否可用或需要配置自定义 token")
	}

	return "", fmt.Errorf("获取封面返回状态码: %d", resp.StatusCode)
}

// DownloadCover 下载封面图片，返回图片字节数据
func (c *Client) DownloadCover(id string) ([]byte, string, error) {
	coverURL, err := c.GetCoverURL(id)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.Get(coverURL)
	if err != nil {
		return nil, "", fmt.Errorf("下载封面图片失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("下载封面图片返回状态码: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取封面图片数据失败: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return data, contentType, nil
}

// extractIDFromURL 从 URL 参数中提取 id
// URL 格式如: https://xxx/api?server=netease&type=url&id=123456&auth=xxx
func extractIDFromURL(apiURL string) string {
	if apiURL == "" {
		return ""
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("id")
}
