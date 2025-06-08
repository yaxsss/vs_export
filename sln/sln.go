package sln

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 添加新的结构体来管理Configuration映射
type ProjectConfigMapping struct {
	ProjectGUID    string
	SolutionConfig string
	ProjectConfig  string
	ShouldBuild    bool
}

type Sln struct {
	SolutionDir    string
	ProjectList    []Project
	ProjectGUIDs   map[string]string      // ProjectPath -> GUID的映射
	ConfigMappings []ProjectConfigMapping // Configuration映射关系
}

func NewSln(path string) (Sln, error) {
	var sln Sln
	var err error

	sln.SolutionDir, err = filepath.Abs(path)
	sln.SolutionDir = filepath.Dir(sln.SolutionDir)
	if err != nil {
		return sln, err
	}

	// 初始化映射
	sln.ProjectGUIDs = make(map[string]string)

	// 解析项目文件和Configuration映射
	err = sln.parseSolutionFile(path)
	if err != nil {
		return sln, err
	}

	// 加载项目文件
	for projectPath := range sln.ProjectGUIDs {
		pro, err := NewProject(filepath.Join(sln.SolutionDir, projectPath))
		if err != nil {
			return sln, err
		}
		sln.ProjectList = append(sln.ProjectList, pro)
	}

	return sln, nil
}

// 解析.sln文件，提取项目和Configuration映射信息
func (sln *Sln) parseSolutionFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}

	content := string(b)

	// 解析项目引用，提取GUID和路径的映射
	err = sln.parseProjectReferences(content)
	if err != nil {
		return err
	}

	// 解析Configuration映射
	err = sln.parseConfigurationMappings(content)
	if err != nil {
		return err
	}

	return nil
}

// 解析项目引用部分
func (sln *Sln) parseProjectReferences(content string) error {
	// 匹配项目引用行：Project("{GUID}") = "ProjectName", "ProjectPath", "{ProjectGUID}"
	re := regexp.MustCompile(`Project\("\{[^}]+\}"\)\s*=\s*"[^"]+",\s*"([^"]+)",\s*"\{([^}]+)\}"`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 3 && strings.HasSuffix(match[1], ".vcxproj") {
			projectPath := strings.Replace(match[1], "\\", "/", -1) // 统一路径分隔符
			projectGUID := match[2]
			sln.ProjectGUIDs[projectPath] = projectGUID
		}
	}

	if len(sln.ProjectGUIDs) == 0 {
		return errors.New("未找到项目文件")
	}

	return nil
}

// 解析Configuration映射部分
func (sln *Sln) parseConfigurationMappings(content string) error {
	// 查找ProjectConfigurationPlatforms部分
	configSectionStart := strings.Index(content, "GlobalSection(ProjectConfigurationPlatforms)")
	if configSectionStart == -1 {
		return nil // 没有Configuration映射也是可以的
	}

	configSectionEnd := strings.Index(content[configSectionStart:], "EndGlobalSection")
	if configSectionEnd == -1 {
		return errors.New("Configuration映射部分格式错误")
	}

	configSection := content[configSectionStart : configSectionStart+configSectionEnd]

	// 匹配Configuration映射行：{GUID}.SolutionConfig.ActiveCfg = ProjectConfig
	// 或：{GUID}.SolutionConfig.Build.0 = ProjectConfig
	re := regexp.MustCompile(`\{([^}]+)\}\.([^.]+)\.(?:ActiveCfg|Build\.0)\s*=\s*(.+?)(?:\r?\n|$)`)
	matches := re.FindAllStringSubmatch(configSection, -1)

	for _, match := range matches {
		if len(match) >= 4 {
			projectGUID := match[1]
			solutionConfig := match[2]
			projectConfig := strings.TrimSpace(match[3]) // 去掉前后空白字符
			shouldBuild := strings.Contains(match[0], "Build.0")

			// 检查是否已存在相同的映射
			found := false
			for i, mapping := range sln.ConfigMappings {
				if mapping.ProjectGUID == projectGUID && mapping.SolutionConfig == solutionConfig {
					// 更新现有映射
					if shouldBuild {
						sln.ConfigMappings[i].ShouldBuild = true
					}
					if sln.ConfigMappings[i].ProjectConfig == "" {
						sln.ConfigMappings[i].ProjectConfig = projectConfig
					}
					found = true
					break
				}
			}

			if !found {
				sln.ConfigMappings = append(sln.ConfigMappings, ProjectConfigMapping{
					ProjectGUID:    projectGUID,
					SolutionConfig: solutionConfig,
					ProjectConfig:  projectConfig,
					ShouldBuild:    shouldBuild,
				})
			}
		}
	}

	return nil
}

// 根据解决方案Configuration查找项目对应的Configuration
func (sln *Sln) GetProjectConfig(projectPath, solutionConfig string) (string, error) {
	// 获取项目的GUID
	projectGUID, exists := sln.ProjectGUIDs[projectPath]
	if !exists {
		return "", fmt.Errorf("项目 %s 未在解决方案中找到", projectPath)
	}

	// 查找Configuration映射
	for _, mapping := range sln.ConfigMappings {
		if mapping.ProjectGUID == projectGUID && mapping.SolutionConfig == solutionConfig {
			// log.Printf("%+v\n", mapping.ProjectConfig)
			return mapping.ProjectConfig, nil
		}
	}

	// 如果没有找到映射，返回默认值（与解决方案Configuration相同）
	return solutionConfig, nil
}

// 根据项目对象查找对应的Configuration
func (sln *Sln) GetProjectConfigByProject(pro *Project, solutionConfig string) (string, error) {
	// 查找该项目在ProjectGUIDs中的路径
	var projectPath string
	for path := range sln.ProjectGUIDs {
		// 比较绝对路径
		absPath := filepath.Join(sln.SolutionDir, path)
		if absPath == pro.ProjectPath {
			projectPath = path
			break
		}
	}

	if projectPath == "" {
		return solutionConfig, nil // 如果找不到，返回默认值
	}

	return sln.GetProjectConfig(projectPath, solutionConfig)
}

// 弃用原来的findAllProject函数，因为我们现在从parseSolutionFile中解析项目
func findAllProject(path string) ([]string, error) {
	// 保留此函数以保持向后兼容，但不再使用
	return []string{}, errors.New("此函数已弃用，请使用新的解析方法")
}

func (sln *Sln) CompileCommandsJson(conf string) ([]CompileCommand, error) {
	var cmdList []CompileCommand

	for _, pro := range sln.ProjectList {
		var item CompileCommand

		// 获取项目对应的Configuration
		projectConfig, err := sln.GetProjectConfigByProject(&pro, conf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: %v, 使用默认Configuration: %s\n", err, conf)
			projectConfig = conf
		}

		for _, f := range pro.FindSourceFiles() {
			item.Dir = pro.ProjectDir
			item.File = f

			inc, def, err := pro.FindConfig(projectConfig)
			if err != nil {
				return cmdList, err
			}
			willReplaceEnv := map[string]string{
				"$(SolutionDir)": sln.SolutionDir,
			}
			for k, v := range willReplaceEnv {
				inc = strings.Replace(inc, k, v, -1)
			}
			def = RemoveBadDefinition(def)
			def = preappend(def, "-D")

			inc = RemoveBadInclude(inc)
			inc = preappend(inc, "-I")

			cmd := "clang-cl.exe " + def + " " + inc + " -c " + f
			item.Cmd = cmd

			cmdList = append(cmdList, item)
		}

	}
	return cmdList, nil
}

func preappend(sepedString string, append string) string {
	defList := strings.Split(sepedString, ";")
	var output string

	for _, v := range defList {
		v = append + v + " "
		output += v
	}
	return output
}
