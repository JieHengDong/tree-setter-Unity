package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FunctionInfo Unity函数信息
type FunctionInfo struct {
	FileName     string
	FilePath     string
	RelativePath string
	Namespace    string
	ClassName    string
	FuncName     string
	Comments     []string
	Signature    string
	IsUnityEvent bool
	IsCoroutine  bool
	Attributes   []string
	Keywords     []string // AI搜索关键词
}

// UnityParser Unity C#解析器
type UnityParser struct {
	xmlCommentRegex    *regexp.Regexp
	singleCommentRegex *regexp.Regexp
	functionRegex      *regexp.Regexp
	namespaceRegex     *regexp.Regexp
	classRegex         *regexp.Regexp
	attributeRegex     *regexp.Regexp
	
	// Unity特定
	unityEvents map[string]bool
}

func NewUnityParser() *UnityParser {
	// Unity常见事件函数
	unityEvents := map[string]bool{
		"Awake": true, "Start": true, "Update": true, "FixedUpdate": true,
		"LateUpdate": true, "OnEnable": true, "OnDisable": true, "OnDestroy": true,
		"OnCollisionEnter": true, "OnCollisionExit": true, "OnCollisionStay": true,
		"OnTriggerEnter": true, "OnTriggerExit": true, "OnTriggerStay": true,
		"OnMouseDown": true, "OnMouseUp": true, "OnMouseEnter": true, "OnMouseExit": true,
		"OnGUI": true, "OnApplicationQuit": true, "OnApplicationPause": true,
		"OnBecameVisible": true, "OnBecameInvisible": true,
	}

	return &UnityParser{
		xmlCommentRegex:    regexp.MustCompile(`///\s*(.+)`),
		singleCommentRegex: regexp.MustCompile(`//\s*(.+)`),
		functionRegex: regexp.MustCompile(
			`(?m)^\s*(?:\[[\w\s,()=.]+\]\s*)*(public|private|protected|internal|static|\s)+([\w<>\[\]]+)\s+(\w+)\s*\(([^)]*)\)`,
		),
		namespaceRegex: regexp.MustCompile(`namespace\s+([\w.]+)`),
		classRegex:     regexp.MustCompile(`(?:public|private|internal)?\s*(?:sealed|abstract)?\s*(?:partial)?\s*class\s+(\w+)`),
		attributeRegex: regexp.MustCompile(`\[(\w+)(?:\([^)]*\))?\]`),
		unityEvents:    unityEvents,
	}
}

// ParseFile 解析单个C#文件
func (p *UnityParser) ParseFile(filePath, rootPath string) ([]FunctionInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var functions []FunctionInfo
	var currentComments []string
	var currentAttributes []string
	var currentNamespace, currentClass string

	// 计算相对路径
	relativePath, _ := filepath.Rel(rootPath, filePath)

	// 提取命名空间和类名
	fullContent := string(content)
	if match := p.namespaceRegex.FindStringSubmatch(fullContent); match != nil {
		currentNamespace = match[1]
	}
	if match := p.classRegex.FindStringSubmatch(fullContent); match != nil {
		currentClass = match[1]
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// 收集特性标记 [SerializeField] [Header("xxx")]
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if match := p.attributeRegex.FindStringSubmatch(trimmed); match != nil {
				currentAttributes = append(currentAttributes, match[1])
			}
			continue
		}

		// 收集注释
		if strings.HasPrefix(trimmed, "///") {
			comment := strings.TrimPrefix(trimmed, "///")
			comment = strings.TrimSpace(comment)
			// 清理XML标签
			comment = cleanXMLTags(comment)
			if comment != "" {
				currentComments = append(currentComments, comment)
			}
			continue
		} else if strings.HasPrefix(trimmed, "//") {
			comment := strings.TrimPrefix(trimmed, "//")
			comment = strings.TrimSpace(comment)
			if comment != "" && !strings.HasPrefix(comment, "/") { // 排除 ////
				currentComments = append(currentComments, comment)
			}
			continue
		}

		// 匹配函数声明
		if match := p.functionRegex.FindStringSubmatch(line); match != nil {
			funcName := match[3]
			returnType := match[2]
			
			// 检查是否是协程
			isCoroutine := strings.Contains(returnType, "IEnumerator")
			
			// 检查是否是Unity事件
			isUnityEvent := p.unityEvents[funcName]

			// 提取关键词
			keywords := extractKeywords(funcName, currentComments)

			funcInfo := FunctionInfo{
				FileName:     filepath.Base(filePath),
				FilePath:     filePath,
				RelativePath: relativePath,
				Namespace:    currentNamespace,
				ClassName:    currentClass,
				FuncName:     funcName,
				Signature:    strings.TrimSpace(line),
				Comments:     make([]string, len(currentComments)),
				Attributes:   make([]string, len(currentAttributes)),
				IsUnityEvent: isUnityEvent,
				IsCoroutine:  isCoroutine,
				Keywords:     keywords,
			}
			copy(funcInfo.Comments, currentComments)
			copy(funcInfo.Attributes, currentAttributes)
			
			functions = append(functions, funcInfo)
			
			currentComments = nil
			currentAttributes = nil
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "[") {
			// 非空行、非注释、非特性，清空缓存
			if !strings.Contains(trimmed, "{") && !strings.Contains(trimmed, "}") {
				currentComments = nil
				currentAttributes = nil
			}
		}
	}

	return functions, nil
}

// cleanXMLTags 清理XML文档注释标签
func cleanXMLTags(s string) string {
	s = regexp.MustCompile(`<summary>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`</summary>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`<param name="[^"]+">([^<]*)</param>`).ReplaceAllString(s, "参数: $1")
	s = regexp.MustCompile(`<returns>([^<]*)</returns>`).ReplaceAllString(s, "返回: $1")
	return strings.TrimSpace(s)
}

// extractKeywords 提取关键词用于AI搜索
func extractKeywords(funcName string, comments []string) []string {
	keywords := []string{}
	
	// 从函数名提取（按驼峰分割）
	funcWords := splitCamelCase(funcName)
	keywords = append(keywords, funcWords...)
	
	// 从注释提取
	commentText := strings.Join(comments, " ")
	// 简单的中文分词（可以集成更专业的分词库）
	words := strings.Fields(commentText)
	for _, word := range words {
		if len(word) > 1 { // 过滤单字
			keywords = append(keywords, strings.ToLower(word))
		}
	}
	
	// 去重
	keywordMap := make(map[string]bool)
	uniqueKeywords := []string{}
	for _, kw := range keywords {
		if !keywordMap[kw] && kw != "" {
			keywordMap[kw] = true
			uniqueKeywords = append(uniqueKeywords, kw)
		}
	}
	
	return uniqueKeywords
}

// splitCamelCase 分割驼峰命名
func splitCamelCase(s string) []string {
	var words []string
	var currentWord strings.Builder
	
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if currentWord.Len() > 0 {
				words = append(words, strings.ToLower(currentWord.String()))
				currentWord.Reset()
			}
		}
		currentWord.WriteRune(r)
	}
	
	if currentWord.Len() > 0 {
		words = append(words, strings.ToLower(currentWord.String()))
	}
	
	return words
}

// ScanUnityProject 扫描Unity项目
func (p *UnityParser) ScanUnityProject(projectPath string) ([]FunctionInfo, error) {
	var allFunctions []FunctionInfo
	
	// Unity项目主要扫描Assets和Packages目录
	assetsPath := filepath.Join(projectPath, "Assets")
	
	if _, err := os.Stat(assetsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("未找到Assets目录，请确认这是Unity项目根目录")
	}

	err := filepath.Walk(assetsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理.cs文件，排除.meta等
		if !info.IsDir() && strings.HasSuffix(path, ".cs") {
			functions, err := p.ParseFile(path, projectPath)
			if err != nil {
				fmt.Printf("⚠️  解析失败 %s: %v\n", path, err)
				return nil
			}
			allFunctions = append(allFunctions, functions...)
		}

		return nil
	})

	return allFunctions, err
}

// GenerateUnityMarkdown 生成Unity优化的Markdown索引
func GenerateUnityMarkdown(functions []FunctionInfo, outputPath string) error {
	var sb strings.Builder

	// 文档头部
	sb.WriteString("# Unity 项目函数索引\n\n")
	sb.WriteString("> 🤖 本文档由AI索引工具自动生成，用于快速定位功能函数\n\n")
	sb.WriteString(fmt.Sprintf("**📊 统计信息**:\n"))
	sb.WriteString(fmt.Sprintf("- 总函数数: %d\n", len(functions)))
	
	// 统计Unity事件和协程
	unityEventCount := 0
	coroutineCount := 0
	for _, fn := range functions {
		if fn.IsUnityEvent {
			unityEventCount++
		}
		if fn.IsCoroutine {
			coroutineCount++
		}
	}
	sb.WriteString(fmt.Sprintf("- Unity生命周期函数: %d\n", unityEventCount))
	sb.WriteString(fmt.Sprintf("- 协程函数: %d\n\n", coroutineCount))
	
	sb.WriteString("---\n\n")

	// 生成快速导航（按分类）
	sb.WriteString("## 🔍 快速导航\n\n")
	
	// 按目录分类
	categoryMap := make(map[string][]FunctionInfo)
	for _, fn := range functions {
		// 提取第一级目录作为分类
		parts := strings.Split(fn.RelativePath, string(filepath.Separator))
		category := "其他"
		if len(parts) > 1 {
			category = parts[0] // Assets后的第一级目录
		}
		categoryMap[category] = append(categoryMap[category], fn)
	}
	
	// 排序分类
	var categories []string
	for cat := range categoryMap {
		categories = append(categories, cat)
	}
	sort.Strings(categories)
	
	for _, cat := range categories {
		anchor := strings.ToLower(strings.ReplaceAll(cat, " ", "-"))
		sb.WriteString(fmt.Sprintf("- [📁 %s (%d)](#%s)\n", cat, len(categoryMap[cat]), anchor))
	}
	
	sb.WriteString("\n---\n\n")

	// 生成详细内容
	for _, category := range categories {
		fns := categoryMap[category]
		
		sb.WriteString(fmt.Sprintf("## 📁 %s\n\n", category))
		sb.WriteString(fmt.Sprintf("> 包含 %d 个函数\n\n", len(fns)))

		// 按类名分组
		classMap := make(map[string][]FunctionInfo)
		for _, fn := range fns {
			className := fn.ClassName
			if className == "" {
				className = "全局函数"
			}
			classMap[className] = append(classMap[className], fn)
		}
		
		// 排序类名
		var classNames []string
		for cn := range classMap {
			classNames = append(classNames, cn)
		}
		sort.Strings(classNames)

		for _, className := range classNames {
			classFns := classMap[className]
			
			sb.WriteString(fmt.Sprintf("### 🔸 类: `%s`\n\n", className))
			
			// 显示文件信息
			if len(classFns) > 0 {
				sb.WriteString(fmt.Sprintf("📄 文件: `%s`\n\n", classFns[0].RelativePath))
			}

			for _, fn := range classFns {
				// 函数标题，带标记
				markers := []string{}
				if fn.IsUnityEvent {
					markers = append(markers, "🎯Unity事件")
				}
				if fn.IsCoroutine {
					markers = append(markers, "⏱️协程")
				}
				
				markerStr := ""
				if len(markers) > 0 {
					markerStr = " " + strings.Join(markers, " ")
				}
				
				sb.WriteString(fmt.Sprintf("#### `%s`%s\n\n", fn.FuncName, markerStr))

				// 特性标记
				if len(fn.Attributes) > 0 {
					sb.WriteString("**特性**: ")
					for i, attr := range fn.Attributes {
						if i > 0 {
							sb.WriteString(", ")
						}
						sb.WriteString(fmt.Sprintf("`[%s]`", attr))
					}
					sb.WriteString("\n\n")
				}

				// 函数签名
				sb.WriteString("```csharp\n")
				sb.WriteString(fn.Signature)
				sb.WriteString("\n```\n\n")

				// 注释说明
				if len(fn.Comments) > 0 {
					sb.WriteString("**📝 说明**:\n")
					for _, comment := range fn.Comments {
						if strings.TrimSpace(comment) != "" {
							sb.WriteString(fmt.Sprintf("> %s\n", comment))
						}
					}
					sb.WriteString("\n")
				}

				// AI搜索关键词
				if len(fn.Keywords) > 0 {
					sb.WriteString("**🔑 关键词**: ")
					// 只显示前8个关键词
					displayKeywords := fn.Keywords
					if len(displayKeywords) > 8 {
						displayKeywords = displayKeywords[:8]
					}
					sb.WriteString("`" + strings.Join(displayKeywords, "` `") + "`")
					sb.WriteString("\n\n")
				}

				sb.WriteString("---\n\n")
			}
		}
	}
	
	// 添加搜索提示
	sb.WriteString("## 💡 使用提示\n\n")
	sb.WriteString("本文档支持以下搜索方式：\n\n")
	sb.WriteString("1. **按功能搜索**: 使用关键词如 \"移动\"、\"攻击\"、\"UI\" 等\n")
	sb.WriteString("2. **按类型搜索**: 搜索 \"Unity事件\"、\"协程\" 等标记\n")
	sb.WriteString("3. **按文件路径搜索**: 使用目录名定位\n")
	sb.WriteString("4. **按类名/函数名搜索**: 直接搜索代码标识符\n\n")
	sb.WriteString("> 💡 提示: 使用 Ctrl+F 在文档中搜索，或将此文档提供给AI助手进行智能查询\n")

	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

// 生成JSON格式（可选，方便其他工具使用）
func GenerateJSON(functions []FunctionInfo, outputPath string) error {
	// 简化版JSON输出
	type SimpleFuncInfo struct {
		Class    string   `json:"class"`
		Function string   `json:"function"`
		File     string   `json:"file"`
		Comments []string `json:"comments"`
		Keywords []string `json:"keywords"`
		IsUnity  bool     `json:"is_unity_event"`
	}
	
	var simplified []SimpleFuncInfo
	for _, fn := range functions {
		simplified = append(simplified, SimpleFuncInfo{
			Class:    fn.ClassName,
			Function: fn.FuncName,
			File:     fn.RelativePath,
			Comments: fn.Comments,
			Keywords: fn.Keywords,
			IsUnity:  fn.IsUnityEvent,
		})
	}
	
	// 这里需要导入 encoding/json
	// 为了保持示例简洁，省略JSON序列化代码
	return nil
}

func main() {
	// 检查命令行参数
	if len(os.Args) < 2 {
		fmt.Println("使用方法: go run main.go <Unity项目路径> [输出文件名]")
		fmt.Println("示例: go run main.go ./MyUnityProject")
		os.Exit(1)
	}

	projectPath := os.Args[1]
	outputFile := "unity-functions-index.md"
	if len(os.Args) >= 3 {
		outputFile = os.Args[2]
	}

	fmt.Println("🚀 开始扫描Unity项目...")
	fmt.Printf("📂 项目路径: %s\n", projectPath)

	parser := NewUnityParser()
	functions, err := parser.ScanUnityProject(projectPath)
	if err != nil {
		fmt.Printf("❌ 扫描失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 找到 %d 个函数\n", len(functions))

	fmt.Println("📝 正在生成Markdown索引...")
	err = GenerateUnityMarkdown(functions, outputFile)
	if err != nil {
		fmt.Printf("❌ 生成失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 索引文档已生成: %s\n", outputFile)
	fmt.Println("\n💡 您现在可以:")
	fmt.Println("   1. 直接在编辑器中搜索关键词")
	fmt.Println("   2. 将文档提供给AI助手进行智能查询")
	fmt.Println("   3. 使用 Ctrl+F 快速定位函数")
}