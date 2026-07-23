//go:build ignore
// +build ignore

package organize

import (
	"context"
	"time"
)

// ============================================================
// 整理模式
// ============================================================

// Mode 整理模式
type Mode int

const (
	// ModePreview 只返回整理后的信息，不执行任何操作
	ModePreview Mode = iota

	// ModeExecute 执行整理操作（重命名、移动、生成STRM）
	ModeExecute
)

// FileOperationMode 文件操作模式
type FileOperationMode int

const (
	// FileOpMove 移动文件（默认行为，同文件系统原子移动，跨文件系统复制+删除）
	FileOpMove FileOperationMode = iota

	// FileOpCopy 复制文件（保留源文件）
	FileOpCopy

	// FileOpHardlink 创建硬连接（同一文件系统不占用额外空间；跨挂载点时退化为复制）
	FileOpHardlink

	// FileOpSoftlink 创建软连接/符号链接（可以跨文件系统，但删除源文件会导致目标失效）
	FileOpSoftlink
)

// String 返回文件操作模式的字符串表示
func (m FileOperationMode) String() string {
	switch m {
	case FileOpMove:
		return "move"
	case FileOpCopy:
		return "copy"
	case FileOpHardlink:
		return "hardlink"
	case FileOpSoftlink:
		return "softlink"
	default:
		return "unknown"
	}
}

// ============================================================
// 整理请求
// ============================================================

// Request 整理请求
type Request struct {
	// 源文件路径（本地路径）
	SourcePath string `json:"sourcePath"`

	// 目标目录（媒体库根目录）
	TargetDir string `json:"targetDir"`

	// 整理模式
	Mode Mode `json:"mode"`

	// 文件操作模式（可选，覆盖全局配置）
	FileOperation FileOperationMode `json:"fileOperation,omitempty"`

	// 是否启用双向同步删除（可选，覆盖全局配置）
	// 启用后：删除源文件会删除目标文件，删除目标文件会删除源文件
	SyncDelete *bool `json:"syncDelete,omitempty"`

	// 是否生成STRM
	GenerateSTRM bool `json:"generateSTRM"`

	// STRM文件内容的URL前缀（如 http://localhost:8091）
	STRMURLPrefix string `json:"strmURLPrefix,omitempty"`

	// 是否刮削元数据
	ScrapeMetadata bool `json:"scrapeMetadata,omitempty"`

	// 命名模板（可选，覆盖全局配置）
	NamingTemplate *NamingTemplate `json:"namingTemplate,omitempty"`

	// 是否覆盖已存在的文件
	Overwrite bool `json:"overwrite,omitempty"`

	// 指定TMDB ID（可选，跳过自动匹配）
	TMDBID *int `json:"tmdbId,omitempty"`

	// 指定媒体类型（可选，movie或tv）
	MediaType *string `json:"mediaType,omitempty"`
}

// ============================================================
// 整理结果
// ============================================================

// Result 整理结果
type Result struct {
	Original *ParsedMedia `json:"original"`

	TMDBMatch *TMDBMatchResult `json:"tmdbMatch,omitempty"`

	NewPath *OrganizedPath `json:"newPath,omitempty"`

	Execution *ExecutionResult `json:"execution,omitempty"`

	Category string `json:"category,omitempty"`
}

// ParsedMedia 解析后的媒体信息
type ParsedMedia struct {
	// 原始文件名
	Original string `json:"original"`

	// 解析出的标题
	Title string `json:"title"`

	// 年份
	Year int `json:"year,omitempty"`

	// 季号（电视剧）
	Season int `json:"season,omitempty"`

	// 集号（电视剧）
	Episode int `json:"episode,omitempty"`

	// 多集支持（如 E01E02）
	Episodes []int `json:"episodes,omitempty"`

	// 分辨率
	Resolution string `json:"resolution,omitempty"`

	// 视频编码
	VideoCodec string `json:"videoCodec,omitempty"`

	// 音频编码
	AudioCodec string `json:"audioCodec,omitempty"`

	// 来源
	Source string `json:"source,omitempty"`

	// 质量
	Quality string `json:"quality,omitempty"`

	// 发布组
	ReleaseGroup string `json:"releaseGroup,omitempty"`

	// 动态范围 (DV, HDR10+, HDR10, HDR, HLG, HDR.Vivid, SDR)
	DynamicRange string `json:"dynamicRange,omitempty"`

	// HQ (高码率)
	HQ bool `json:"hq,omitempty"`

	// FPS (帧率)
	FPS int `json:"fps,omitempty"`

	// Part (分片标识: Part1, CD2, 上/下)
	Part string `json:"part,omitempty"`

	// 是否是电影
	IsMovie bool `json:"isMovie"`

	// 文件扩展名
	Extension string `json:"extension"`
}

// TMDBMatchResult TMDB匹配结果
type TMDBMatchResult struct {
	// 是否匹配成功
	Matched bool `json:"matched"`

	// TMDB ID
	TMDBID int `json:"tmdbId,omitempty"`

	// 媒体类型: movie, tv
	MediaType string `json:"mediaType,omitempty"`

	// 标题
	Title string `json:"title,omitempty"`

	// 原始标题
	OriginalTitle string `json:"originalTitle,omitempty"`

	// 年份
	Year int `json:"year,omitempty"`

	// 简介
	Overview string `json:"overview,omitempty"`

	// 海报路径
	PosterPath string `json:"posterPath,omitempty"`

	// 背景图路径
	BackdropPath string `json:"backdropPath,omitempty"`

	// 匹配置信度 0-1
	Confidence float64 `json:"confidence,omitempty"`

	// TMDB 评分
	VoteAverage float64 `json:"voteAverage,omitempty"`

	// 电视剧总集数（仅 TV）
	TotalEpisodes int `json:"totalEpisodes,omitempty"`

	// 类型ID列表（用于分类）
	GenreIDs []string `json:"genreIds,omitempty"`

	// 原始语言
	OriginalLanguage string `json:"originalLanguage,omitempty"`

	// 产地国家
	OriginCountries []string `json:"originCountries,omitempty"`
}

// OrganizedPath 整理后的路径
type OrganizedPath struct {
	// 完整的目标文件路径
	FullPath string `json:"fullPath"`

	// 目录路径
	DirPath string `json:"dirPath"`

	// 文件名（不含目录）
	FileName string `json:"fileName"`

	// STRM文件路径（如果需要生成）
	STRMPath string `json:"strmPath,omitempty"`
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	// 是否成功
	Success bool `json:"success"`

	// 原路径
	OldPath string `json:"oldPath"`

	// 新路径
	NewPath string `json:"newPath"`

	// 是否生成了STRM
	STRMGenerated bool `json:"strmGenerated,omitempty"`

	// STRM路径
	STRMPath string `json:"strmPath,omitempty"`

	// 错误信息
	Error string `json:"error,omitempty"`
}

// ============================================================
// 批量规划相关类型
// ============================================================

// FileInput 表示一个待整理的文件输入
type FileInput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size,omitempty"`
}

// PlanRequest 规划请求（Preview 和 Execute 共用）
type PlanRequest struct {
	Files          []FileInput       `json:"files"`
	TargetDir      string            `json:"targetDir"`
	ResourceName   string            `json:"resourceName,omitempty"`
	Year           string            `json:"year,omitempty"`
	TMDBID         *int              `json:"tmdbId,omitempty"`
	MediaType      *string           `json:"mediaType,omitempty"`
	TMDBMatch      *TMDBMatchResult  `json:"tmdbMatch,omitempty"` // pre-matched result; skips getByID API call
	Mode           Mode              `json:"mode"`
	FileOperation  FileOperationMode `json:"fileOperation,omitempty"`
	SyncDelete     *bool             `json:"syncDelete,omitempty"`
	GenerateSTRM   bool              `json:"generateSTRM,omitempty"`
	STRMURLPrefix  string            `json:"strmURLPrefix,omitempty"`
	Overwrite      bool              `json:"overwrite,omitempty"`
	Scrape         bool              `json:"scrape,omitempty"`
	NamingTemplate *NamingTemplate   `json:"namingTemplate,omitempty"`
}

// PlanResult 规划结果
type PlanResult struct {
	Success bool           `json:"success"`
	Error   string         `json:"error,omitempty"`
	Items   []PlanItem     `json:"items"`
	Media   map[string]any `json:"media,omitempty"`
}

// PlanItem 规划项（一个文件的整理计划）
type PlanItem struct {
	FileInput   FileInput        `json:"fileInput"`
	Parsed      *ParsedMedia     `json:"parsed,omitempty"`
	TMDBMatch   *TMDBMatchResult `json:"tmdbMatch,omitempty"`
	TargetPath  string           `json:"targetPath,omitempty"`
	Category    string           `json:"category,omitempty"`
	Conflict    bool             `json:"conflict,omitempty"`
	ConflictMsg string           `json:"conflictMsg,omitempty"`
	Execution   *ExecutionResult `json:"execution,omitempty"`
}

// ScanGroup 文件分组结果
type ScanGroup struct {
	ResourceName string      `json:"resourceName"`
	Files        []FileInput `json:"files"`
}

// ============================================================
// TMDB 刮削相关类型
// ============================================================

// TMDBDetailProvider TMDB 详情数据提供者接口
type TMDBDetailProvider interface {
	GetMovieDetails(ctx context.Context, tmdbID int) (*MovieScrapeInfo, error)
	GetTVDetails(ctx context.Context, tmdbID int) (*TVScrapeInfo, error)
	GetTVEpisodeDetails(ctx context.Context, tmdbID, seasonNum, episodeNum int) (*EpisodeScrapeInfo, error)
}

// MovieScrapeInfo 电影刮削信息（Emby/Jellyfin NFO 兼容）
type MovieScrapeInfo struct {
	Title         string     `json:"title"`
	OriginalTitle string     `json:"originalTitle"`
	Overview      string     `json:"overview"`
	ReleaseDate   string     `json:"releaseDate"`
	PosterPath    string     `json:"posterPath,omitempty"`
	BackdropPath  string     `json:"backdropPath,omitempty"`
	LogoPath      string     `json:"logoPath,omitempty"`
	VoteAverage   float64    `json:"voteAverage"`
	TMDBID        int        `json:"tmdbId"`
	Cast          []CastInfo `json:"cast,omitempty"`
}

// TVScrapeInfo 电视剧刮削信息
type TVScrapeInfo struct {
	Title         string     `json:"title"`
	OriginalTitle string     `json:"originalTitle"`
	Overview      string     `json:"overview"`
	ReleaseDate   string     `json:"releaseDate"`
	PosterPath    string     `json:"posterPath,omitempty"`
	BackdropPath  string     `json:"backdropPath,omitempty"`
	LogoPath      string     `json:"logoPath,omitempty"`
	VoteAverage   float64    `json:"voteAverage"`
	TMDBID        int        `json:"tmdbId"`
	TotalSeasons  int        `json:"totalSeasons,omitempty"`
	Status        string     `json:"status,omitempty"`
	Cast          []CastInfo `json:"cast,omitempty"`
}

// CastInfo 演员信息
type CastInfo struct {
	Name        string `json:"name"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profilePath,omitempty"`
}

// EpisodeScrapeInfo 剧集单集刮削信息
type EpisodeScrapeInfo struct {
	Title       string  `json:"title"`
	Overview    string  `json:"overview"`
	AirDate     string  `json:"airDate"`
	StillPath   string  `json:"stillPath,omitempty"`
	VoteAverage float64 `json:"voteAverage"`
	Season      int     `json:"season"`
	Episode     int     `json:"episode"`
}

// ============================================================
// 命名模板
// ============================================================

// NamingTemplate 命名模板
type NamingTemplate struct {
	Movie string `json:"movie,omitempty" yaml:"movie"`

	TVShowDir string `json:"tvShowDir,omitempty" yaml:"tvShowDir"`

	TVSeasonDir string `json:"tvSeasonDir,omitempty" yaml:"tvSeasonDir"`

	TVFile string `json:"tvFile,omitempty" yaml:"tvFile"`

	EnableCategory bool `json:"enableCategory,omitempty" yaml:"enable_category"`
}

// ============================================================
// 目录监控
// ============================================================

// WatchOptions 目录监控选项
type WatchOptions struct {
	// 监控的本地目录路径
	Directory string `json:"directory"`

	// 整理目标目录
	TargetDir string `json:"targetDir"`

	// 整理模式（通常用ModeExecute）
	Mode Mode `json:"mode"`

	// 文件操作模式（可选，覆盖全局配置）
	FileOperation FileOperationMode `json:"fileOperation,omitempty"`

	// 是否启用双向同步删除（可选，覆盖全局配置）
	SyncDelete *bool `json:"syncDelete,omitempty"`

	// 是否递归监控子目录
	Recursive bool `json:"recursive,omitempty"`

	// 是否启用分类（可选，覆盖全局配置）
	EnableCategory *bool `json:"enableCategory,omitempty"`

	// 命名模板（可选，覆盖全局配置）
	NamingTemplate *NamingTemplate `json:"namingTemplate,omitempty"`

	// 是否启用刮削元数据
	EnableScrape *bool `json:"enableScrape,omitempty"`

	// 是否覆盖已存在的目标文件（默认: false）
	Overwrite bool `json:"overwrite,omitempty"`

	// 文件过滤（只处理媒体文件）
	FileFilter func(filename string) bool `json:"-"`

	// 整理完成回调
	OnComplete func(result *Result) `json:"-"`

	// 错误处理回调
	OnError func(err error) `json:"-"`

	// 文件稳定等待时间（默认5秒）
	StableWait time.Duration `json:"stableWait,omitempty"`

	// 文件写入检测间隔（默认1秒）
	StableCheckInterval time.Duration `json:"stableCheckInterval,omitempty"`

	// 整理完成后回调 URL（POST JSON）
	CallbackURL string `json:"callbackUrl,omitempty"`
}

// ============================================================
// 配置
// ============================================================

// Config 整理器配置
type Config struct {
	// TMDB API Key
	TMDBAPIKey string `json:"tmdbApiKey"`

	// TMDB 语言
	TMDBLanguage string `json:"tmdbLanguage,omitempty"` // 默认: zh-CN

	// TMDB API 加速地址（可选）
	// 例如: https://api.tmdb.org/3 或自定义加速域名
	// 留空则使用默认地址: https://api.themoviedb.org/3
	TMDBBaseURL string `json:"tmdbBaseUrl,omitempty"`

	// 命名模板
	NamingTemplate *NamingTemplate `json:"namingTemplate,omitempty"`

	// 分类配置文件路径
	CategoryConfigPath string `json:"categoryConfigPath,omitempty"`

	// 文件操作模式（默认: FileOpMove）
	FileOperation FileOperationMode `json:"fileOperation,omitempty"`

	// 是否启用双向同步删除（默认: false）
	// 启用后：删除源文件会删除目标文件，删除目标文件会删除源文件
	SyncDelete bool `json:"syncDelete,omitempty"`

	// 媒体文件扩展名
	MediaExtensions []string `json:"mediaExtensions,omitempty"`

	// 字幕文件扩展名
	SubtitleExtensions []string `json:"subtitleExtensions,omitempty"`

	// DetailProvider TMDB 详情提供者（可选，用于刮削）
	DetailProvider TMDBDetailProvider `json:"-"`

	// HTTPClient 用于刮削图片下载（可选）
	HTTPClient interface{} `json:"-"`

	// ImageDownloadConcurrency 图片下载并发数（默认: 4）
	ImageDownloadConcurrency int `json:"imageDownloadConcurrency,omitempty"`

	// ImageDownloadQueueSize 图片下载队列长度（默认: 并发数 * 8）
	ImageDownloadQueueSize int `json:"imageDownloadQueueSize,omitempty"`

	// ImageDownloadTimeoutSeconds 图片下载超时（秒，默认: 15）
	ImageDownloadTimeoutSeconds int `json:"imageDownloadTimeoutSeconds,omitempty"`
}

// ============================================================
// Organizer 接口
// ============================================================

// Organizer 整理器接口
type Organizer interface {
	// ===== 解析能力 =====

	// ParseFilename 解析文件名，提取技术信息
	// 输入: "北上.S01E01.1080p.WEB-DL.H264.DDP5.1-Group.mp4"
	// 输出: ParsedMedia 结构体
	ParseFilename(filename string) (*ParsedMedia, error)

	// MatchTMDB 根据解析结果匹配TMDB
	// 支持：标题+年份、标题+季集、TMDB ID直接指定
	MatchTMDB(ctx context.Context, parsed *ParsedMedia) (*TMDBMatchResult, error)

	// ===== 路径生成 =====

	// GeneratePath 根据TMDB信息和模板生成整理后的路径
	GeneratePath(match *TMDBMatchResult, parsed *ParsedMedia, template *NamingTemplate) (*OrganizedPath, string, error)

	// ===== 整理执行 =====

	// Organize 整理资源（支持Preview和Execute模式）
	Organize(ctx context.Context, req *Request) (*Result, error)

	// OrganizeBatch 批量整理
	OrganizeBatch(ctx context.Context, reqs []*Request) ([]*Result, error)

	// ===== 目录监控 =====

	// WatchDirectory 监控本地目录变化，自动整理
	WatchDirectory(ctx context.Context, opts WatchOptions) error

	// StopWatch 停止目录监控
	StopWatch(directory string) error

	// ===== 批量规划 =====

	// Plan 规划整理方案（解析+匹配+路径生成+冲突检测，不执行移动）
	Plan(ctx context.Context, req *PlanRequest) (*PlanResult, error)

	// BatchScanOrganize 批量扫描目录并按 watcher 路径整理（applyDirectoryHints + 局部 cache）
	BatchScanOrganize(ctx context.Context, jobID string, opts ScanOrganizeOptions) error

	Scrape(ctx context.Context, targetPath string, tmdbID int, mediaType string, season, episode int, episodeFileName string) error

	// ReloadCategory 从 YAML 字节热加载分类规则
	ReloadCategory(data []byte) error

	// ReloadCategoryFromFile 从文件热加载分类规则
	ReloadCategoryFromFile(configPath string) error

	// Close 释放资源（停止所有 watcher、关闭 TMDB 连接）
	Close()
}
