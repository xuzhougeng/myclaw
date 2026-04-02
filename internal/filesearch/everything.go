package filesearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"baize/internal/toolcontract"
)

const (
	ToolName         = "everything_file_search"
	DefaultLimit     = 10
	ShortcutName     = "/find"
	SendShortcutName = "/send"
)

var (
	ErrUnsupported  = errors.New("当前仅 Windows 支持 Everything 文件检索，macOS/Linux 暂未实现")
	ErrUnconfigured = errors.New("请先配置 es.exe 路径")
)

var (
	everythingRelativeDayPattern      = regexp.MustCompile(`^(last|past|prev|coming|next)(\d+)days?$`)
	everythingChineseRecentDayPattern = regexp.MustCompile(`^(?:近|最近|这)(\d+)天$`)
	everythingDrivePattern            = regexp.MustCompile(`(?i)^([a-z])(?::|盘)?$`)
)

type ToolInput struct {
	Query        string   `json:"query,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	Drives       []string `json:"drives,omitempty"`
	KnownFolders []string `json:"known_folders,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	Extensions   []string `json:"extensions,omitempty"`
	DateField    string   `json:"date_field,omitempty"`
	DateValue    string   `json:"date_value,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

type ResultItem struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Path  string `json:"path"`
}

type ToolResult struct {
	Tool  string       `json:"tool"`
	Query string       `json:"query"`
	Limit int          `json:"limit"`
	Count int          `json:"count"`
	Items []ResultItem `json:"items"`
}

type executionPlan struct {
	display string
	args    []string
}

func Definition() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              ToolName,
		Purpose:           "Search existing local files on Windows through Everything (es.exe).",
		Description:       "Search Windows local files indexed by Everything. The tool accepts either a native Everything query or structured semantic filters and returns normalized file hits.",
		InputContract:     "query overrides semantic fields; otherwise provide any combination of keywords, drives, known_folders, paths, extensions, date_field, date_value, and limit.",
		OutputContract:    "Returns the executed search form, effective limit, result count, and ordered file items with index, name, and full path.",
		InputJSONExample:  `{"known_folders":["downloads"],"extensions":["pdf"],"date_field":"created","date_value":"last48hours","limit":10}`,
		OutputJSONExample: `{"tool":"everything_file_search","query":"file: shell:Downloads *.pdf dc:last48hours","limit":10,"count":1,"items":[{"index":1,"name":"单细胞.pdf","path":"C:\\Users\\demo\\Downloads\\单细胞.pdf"}]}`,
		Usage:             UsageText(),
	}
}

func UsageText() string {
	return strings.TrimSpace(`
Tool: everything_file_search
Purpose: search existing local files on Windows through Everything (es.exe).

Input:
- query: optional native Everything query. Advanced/manual mode. If provided, it overrides semantic fields below.
- keywords: real topic words, for example ["单细胞"].
- drives: drive letters without colon, for example ["d"].
- known_folders: one or more of downloads, desktop, documents, pictures, music, videos.
- paths: explicit Windows paths such as D:\reports or E:\xwechat_files.
- extensions: file extensions without dots, for example ["pdf"] or ["doc","docx"].
- date_field: modified, created, or recentchange.
- date_value: Everything-style date constant such as today, yesterday, thisweek, thismonth, last48hours.
- limit: max number of files to return. Default is 10.

Output:
- tool: always everything_file_search.
- query: the final Everything query that was executed.
- limit: the effective result limit.
- count: number of matched files returned.
- items: [{index, name, path}] in user-facing selection order.

Query notes:
- The tool searches files only and filters out directories.
- Common folder aliases are normalized, for example downloads -> shell:Downloads.
- Relative day windows are normalized to hours, for example last2days -> last48hours.
`)
}

func CommandHelpText() string {
	return strings.TrimSpace(`
` + ShortcutName + ` 是 ` + ToolName + ` 的快捷入口。

快捷用法:
- ` + ShortcutName + ` help
- ` + ShortcutName + ` <原生 Everything 检索式>

自然语言也可以直接触发这个模块，例如:
- 查找下载目录下的 pdf 文件
- 查找 D 盘单细胞相关的 PDF 文件
- 查找下载目录下这两天下载的文件

模块输入:
- 原生模式: 直接给 Everything query，例如 ` + "`file: shell:Downloads *.pdf`" + `
- 结构化模式: keywords / drives / known_folders / paths / extensions / date_field / date_value / limit

模块输出:
- 返回前 N 个文件候选，包含编号、文件名、完整路径
- 在微信里回复编号即可发送对应文件

常用约束:
- 下载目录: ` + "`shell:Downloads`" + `
- D 盘: ` + "`D:\\`" + `
- PDF: ` + "`*.pdf`" + `
- 今天修改: ` + "`dm:today`" + `
- 近两天新建: ` + "`dc:last48hours`" + `

微信发送:
- 先用 ` + ShortcutName + ` 查找候选文件
- 再用 ` + SendShortcutName + ` <序号> 发送对应文件
`)
}

func NormalizeInput(raw ToolInput) ToolInput {
	normalized := ToolInput{
		Query:     strings.Join(strings.Fields(strings.TrimSpace(raw.Query)), " "),
		DateField: normalizeDateField(raw.DateField),
		DateValue: normalizeDateValue(raw.DateValue),
		Limit:     raw.Limit,
	}

	keywordSeen := make(map[string]struct{})
	driveSeen := make(map[string]struct{})
	folderSeen := make(map[string]struct{})
	pathSeen := make(map[string]struct{})
	extSeen := make(map[string]struct{})

	appendKeyword := func(value string) {
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := keywordSeen[key]; ok {
			return
		}
		keywordSeen[key] = struct{}{}
		normalized.Keywords = append(normalized.Keywords, value)
	}
	appendDrive := func(value string) {
		if value == "" {
			return
		}
		if _, ok := driveSeen[value]; ok {
			return
		}
		driveSeen[value] = struct{}{}
		normalized.Drives = append(normalized.Drives, value)
	}
	appendFolder := func(value string) {
		if value == "" {
			return
		}
		if _, ok := folderSeen[value]; ok {
			return
		}
		folderSeen[value] = struct{}{}
		normalized.KnownFolders = append(normalized.KnownFolders, value)
	}
	appendPath := func(value string) {
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := pathSeen[key]; ok {
			return
		}
		pathSeen[key] = struct{}{}
		normalized.Paths = append(normalized.Paths, value)
	}
	appendExt := func(value string) {
		if value == "" {
			return
		}
		if _, ok := extSeen[value]; ok {
			return
		}
		extSeen[value] = struct{}{}
		normalized.Extensions = append(normalized.Extensions, value)
	}

	for _, item := range raw.Drives {
		appendDrive(normalizeDrive(item))
	}
	for _, item := range raw.KnownFolders {
		appendFolder(normalizeKnownFolder(item))
	}
	for _, item := range raw.Paths {
		value := normalizePath(item)
		switch {
		case value == "":
		case strings.HasSuffix(value, ":"):
			appendDrive(normalizeDrive(value))
		case normalizeKnownFolder(value) != "":
			appendFolder(normalizeKnownFolder(value))
		default:
			appendPath(value)
		}
	}
	for _, item := range raw.Extensions {
		for _, ext := range normalizeExtensions(item) {
			appendExt(ext)
		}
	}
	for _, item := range raw.Keywords {
		value := normalizeKeyword(item)
		if value == "" {
			continue
		}
		switch {
		case normalizeKnownFolder(value) != "":
			appendFolder(normalizeKnownFolder(value))
		case normalizeDrive(value) != "":
			appendDrive(normalizeDrive(value))
		default:
			exts := normalizeExtensions(value)
			if len(exts) > 0 {
				for _, ext := range exts {
					appendExt(ext)
				}
				continue
			}
			appendKeyword(value)
		}
	}
	if normalized.DateField == "" && normalized.DateValue != "" {
		normalized.DateField = "modified"
	}
	if normalized.Limit <= 0 {
		normalized.Limit = DefaultLimit
	}
	return normalized
}

func CompileQuery(input ToolInput) string {
	return compileQuery(input, true, false)
}

func DescribeExecution(input ToolInput) string {
	return buildExecutionPlan(input).display
}

func compileQuery(input ToolInput, includeLocation bool, allowFileOnly bool) string {
	input = NormalizeInput(input)
	if input.Query != "" {
		return input.Query
	}

	parts := []string{"file:"}
	constraintCount := 0

	if includeLocation {
		if driveFilter := buildORFilter(buildDriveTerms(input.Drives)); driveFilter != "" {
			parts = append(parts, driveFilter)
			constraintCount++
		}
		if folderFilter := buildORFilter(buildKnownFolderTerms(input.KnownFolders)); folderFilter != "" {
			parts = append(parts, folderFilter)
			constraintCount++
		}
		if pathFilter := buildORFilter(buildPathTerms(input.Paths)); pathFilter != "" {
			parts = append(parts, pathFilter)
			constraintCount++
		}
	}
	if extFilter := buildORFilter(buildExtensionTerms(input.Extensions)); extFilter != "" {
		parts = append(parts, extFilter)
		constraintCount++
	}
	if dateFilter := buildDateFilter(input.DateField, input.DateValue); dateFilter != "" {
		parts = append(parts, dateFilter)
		constraintCount++
	}
	for _, keyword := range input.Keywords {
		token := quoteToken(keyword)
		if token == "" {
			continue
		}
		parts = append(parts, token)
		constraintCount++
	}

	if constraintCount == 0 && !allowFileOnly {
		return ""
	}
	return strings.Join(parts, " ")
}

func buildExecutionPlan(input ToolInput) executionPlan {
	input = NormalizeInput(input)
	if input.Query != "" {
		return executionPlan{
			display: input.Query,
			args:    []string{"-n", strconv.Itoa(input.Limit), input.Query},
		}
	}

	if scopePath := executionScopePath(input); scopePath != "" {
		searchInput := input
		searchInput.Drives = nil
		searchInput.Paths = nil
		searchText := compileQuery(searchInput, false, true)
		args := []string{"-n", strconv.Itoa(input.Limit), "-path", scopePath}
		display := "-path " + scopePath
		if searchText != "" {
			args = append(args, searchText)
			display += " " + searchText
		}
		return executionPlan{display: display, args: args}
	}

	query := compileQuery(input, true, false)
	if query == "" {
		return executionPlan{}
	}
	return executionPlan{
		display: query,
		args:    []string{"-n", strconv.Itoa(input.Limit), query},
	}
}

func executionScopePath(input ToolInput) string {
	if len(input.KnownFolders) != 0 {
		return ""
	}
	switch {
	case len(input.Drives) == 1 && len(input.Paths) == 0:
		return strings.ToUpper(input.Drives[0]) + `:\`
	case len(input.Paths) == 1 && len(input.Drives) == 0:
		return normalizePath(input.Paths[0])
	default:
		return ""
	}
}

func ExecuteWithEverything(ctx context.Context, everythingPath string, input ToolInput) (ToolResult, error) {
	if runtime.GOOS != "windows" {
		return ToolResult{}, ErrUnsupported
	}

	commandPath := strings.Trim(strings.TrimSpace(everythingPath), "\"")
	if commandPath == "" {
		return ToolResult{}, ErrUnconfigured
	}

	input = NormalizeInput(input)
	plan := buildExecutionPlan(input)
	if len(plan.args) == 0 {
		return ToolResult{
			Tool:  ToolName,
			Query: "",
			Limit: input.Limit,
			Items: nil,
		}, nil
	}

	searchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	output, err := exec.CommandContext(searchCtx, commandPath, plan.args...).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ToolResult{}, fmt.Errorf("%w: %s", ErrUnconfigured, commandPath)
		}
		return ToolResult{}, fmt.Errorf("执行 es.exe 失败: %w", err)
	}

	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	items := make([]ResultItem, 0, input.Limit)
	seen := make(map[string]struct{})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key := strings.ToLower(line)
		if _, ok := seen[key]; ok {
			continue
		}
		info, statErr := os.Stat(line)
		if statErr != nil || info.IsDir() {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, ResultItem{
			Index: len(items) + 1,
			Name:  fileBaseName(line),
			Path:  line,
		})
		if len(items) >= input.Limit {
			break
		}
	}

	return ToolResult{
		Tool:  ToolName,
		Query: plan.display,
		Limit: input.Limit,
		Count: len(items),
		Items: items,
	}, nil
}

func buildDriveTerms(drives []string) []string {
	terms := make([]string, 0, len(drives))
	for _, drive := range drives {
		drive = normalizeDrive(drive)
		if drive == "" {
			continue
		}
		terms = append(terms, strings.ToUpper(drive)+`:\`)
	}
	return terms
}

func buildKnownFolderTerms(folders []string) []string {
	terms := make([]string, 0, len(folders))
	for _, folder := range folders {
		name := knownFolderName(folder)
		if name == "" {
			continue
		}
		terms = append(terms, "shell:"+name)
	}
	return terms
}

func buildPathTerms(paths []string) []string {
	terms := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := normalizePath(raw)
		if path == "" {
			continue
		}
		terms = append(terms, quoteToken(path))
	}
	return terms
}

func buildExtensionTerms(extensions []string) []string {
	terms := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		if ext == "" {
			continue
		}
		terms = append(terms, "*."+ext)
	}
	return terms
}

func buildDateFilter(field, value string) string {
	if value == "" {
		return ""
	}
	switch normalizeDateField(field) {
	case "modified":
		return "dm:" + value
	case "created":
		return "dc:" + value
	case "recentchange":
		return "rc:" + value
	default:
		return ""
	}
}

func buildORFilter(terms []string) string {
	switch len(terms) {
	case 0:
		return ""
	case 1:
		return terms[0]
	default:
		return "<" + strings.Join(terms, "|") + ">"
	}
}

func normalizeDateField(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "modified", "modify", "updated", "update", "修改", "更新":
		return "modified"
	case "created", "create", "new", "downloaded", "generated", "生成", "新建", "下载":
		return "created"
	case "recentchange", "recent", "changed", "change", "变化":
		return "recentchange"
	default:
		return ""
	}
}

func normalizeDateValue(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "-", "")
	switch value {
	case "", "none", "null":
		return ""
	case "currentweek":
		return "thisweek"
	case "currentmonth":
		return "thismonth"
	case "currentyear":
		return "thisyear"
	case "今天":
		return "today"
	case "昨天":
		return "yesterday"
	case "本周", "这周", "本星期", "这星期":
		return "thisweek"
	case "上周", "上一周", "上星期":
		return "lastweek"
	case "本月", "这个月":
		return "thismonth"
	case "上月", "上个月":
		return "lastmonth"
	case "这两天", "近两天", "最近两天":
		return "last48hours"
	case "这三天", "近三天", "最近三天":
		return "last72hours"
	}
	if matches := everythingRelativeDayPattern.FindStringSubmatch(value); len(matches) == 3 {
		days, err := strconv.Atoi(matches[2])
		if err == nil && days > 0 {
			return matches[1] + strconv.Itoa(days*24) + "hours"
		}
	}
	if matches := everythingChineseRecentDayPattern.FindStringSubmatch(value); len(matches) == 2 {
		days, err := strconv.Atoi(matches[1])
		if err == nil && days > 0 {
			return "last" + strconv.Itoa(days*24) + "hours"
		}
	}
	return value
}

func normalizeDrive(raw string) string {
	matches := everythingDrivePattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 2 {
		return ""
	}
	return strings.ToLower(matches[1])
}

func normalizeKnownFolder(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, " ", "")
	switch value {
	case "downloads", "download", "下载", "下载目录", "下载文件夹":
		return "downloads"
	case "desktop", "桌面":
		return "desktop"
	case "documents", "document", "docs", "文档", "我的文档", "文稿":
		return "documents"
	case "pictures", "picture", "photos", "photo", "图片", "照片":
		return "pictures"
	case "music", "音乐", "音频":
		return "music"
	case "videos", "video", "视频":
		return "videos"
	default:
		return ""
	}
}

func knownFolderName(raw string) string {
	switch normalizeKnownFolder(raw) {
	case "downloads":
		return "Downloads"
	case "desktop":
		return "Desktop"
	case "documents":
		return "Documents"
	case "pictures":
		return "Pictures"
	case "music":
		return "Music"
	case "videos":
		return "Videos"
	default:
		return ""
	}
}

func normalizePath(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, `"'`))
	if value == "" {
		return ""
	}
	if folder := normalizeKnownFolder(value); folder != "" {
		return folder
	}
	if drive := normalizeDrive(value); drive != "" {
		return drive + ":"
	}
	value = strings.ReplaceAll(value, "/", `\`)
	if len(value) >= 2 && value[1] == ':' {
		value = strings.ToLower(value[:1]) + value[1:]
	}
	if looksLikeDirectoryPath(value) && !strings.HasSuffix(value, `\`) {
		value += `\`
	}
	return value
}

func looksLikeDirectoryPath(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasSuffix(value, `\`) {
		return true
	}
	if len(value) >= 2 && value[1] == ':' {
		lastSep := strings.LastIndex(value, `\`)
		if lastSep < 0 {
			return true
		}
		lastPart := value[lastSep+1:]
		return !strings.Contains(lastPart, ".")
	}
	return false
}

func normalizeExtensions(raw string) []string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimPrefix(value, ".")
	value = strings.TrimSuffix(value, "文件")
	value = strings.TrimSuffix(value, "格式")
	switch value {
	case "", "文件":
		return nil
	case "pdf":
		return []string{"pdf"}
	case "doc", "docx", "word", "word文档", "文档", "文稿":
		return []string{"doc", "docx"}
	case "xls", "xlsx", "excel", "excel表格", "表格":
		return []string{"xls", "xlsx"}
	case "ppt", "pptx", "powerpoint", "幻灯片":
		return []string{"ppt", "pptx"}
	case "txt", "文本":
		return []string{"txt"}
	case "csv":
		return []string{"csv"}
	case "md", "markdown":
		return []string{"md"}
	case "json":
		return []string{"json"}
	case "yaml", "yml":
		return []string{"yaml", "yml"}
	case "toml":
		return []string{"toml"}
	case "ini":
		return []string{"ini"}
	case "log":
		return []string{"log"}
	case "jpg", "jpeg", "png", "gif", "webp", "bmp":
		return []string{value}
	case "zip", "rar", "7z", "tar", "gz":
		return []string{value}
	case "mp3", "wav", "flac":
		return []string{value}
	case "mp4", "mov", "avi", "mkv":
		return []string{value}
	case "go", "py", "js", "ts", "tsx", "jsx", "java", "c", "cc", "cpp", "h", "hpp", "rs", "sh":
		return []string{value}
	default:
		return nil
	}
}

func normalizeKeyword(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, `"'`))
	value = strings.Trim(value, "，。,.；;：:！？!?()[]{}<>")
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	switch strings.ToLower(value) {
	case "帮我", "查找", "搜索", "找", "查一下", "搜一下", "文件", "文件夹", "目录", "相关", "有关":
		return ""
	}
	if normalizeKnownFolder(value) != "" || normalizeDrive(value) != "" {
		return ""
	}
	if len(normalizeExtensions(value)) > 0 {
		return ""
	}
	return value
}

func quoteToken(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}

func fileBaseName(filePath string) string {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "file"
	}
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	lastSlash := strings.LastIndex(normalized, "/")
	if lastSlash >= 0 {
		normalized = normalized[lastSlash+1:]
	}
	if normalized == "" || normalized == "." || normalized == "/" {
		return "file"
	}
	return normalized
}
