package configuration

import (
	"errors"
	"fmt"
	"github.com/diggerhq/lib-digger-config/terragrunt/atlantis"
	"github.com/dominikbraun/graph"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type DirWalker interface {
	GetDirs(workingDir string) ([]string, error)
}

type FileSystemTopLevelTerraformDirWalker struct {
}

type FileSystemTerragruntDirWalker struct {
}

type FileSystemModuleDirWalker struct {
}

func GetFilesWithExtension(workingDir string, ext string) ([]string, error) {
	var files []string
	listOfFiles, err := os.ReadDir(workingDir)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error reading directory %s: %v", workingDir, err))
	}
	for _, f := range listOfFiles {
		if !f.IsDir() {
			r, err := regexp.MatchString(ext, f.Name())
			if err == nil && r {
				files = append(files, f.Name())
			}
		}
	}

	return files, nil
}

func (walker *FileSystemTopLevelTerraformDirWalker) GetDirs(workingDir string) ([]string, error) {
	var dirs []string
	err := filepath.Walk(workingDir,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "modules" {
					return filepath.SkipDir
				}
				terraformFiles, _ := GetFilesWithExtension(path, ".tf")
				if len(terraformFiles) > 0 {
					dirs = append(dirs, strings.ReplaceAll(path, workingDir+string(os.PathSeparator), ""))
					return filepath.SkipDir
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

func (walker *FileSystemModuleDirWalker) GetDirs(workingDir string) ([]string, error) {
	var dirs []string
	err := filepath.Walk(workingDir,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}
			if info.IsDir() && info.Name() == "modules" {
				dirs = append(dirs, strings.ReplaceAll(path, workingDir+string(os.PathSeparator), ""))
				return filepath.SkipDir
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

func (walker *FileSystemTerragruntDirWalker) GetDirs(workingDir string) ([]string, error) {
	var dirs []string
	err := filepath.Walk(workingDir,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "modules" {
					return filepath.SkipDir
				}
				terragruntFiles, _ := GetFilesWithExtension(path, "terragrunt.hcl")
				if len(terragruntFiles) > 0 {
					for _, f := range terragruntFiles {
						terragruntFile := path + string(os.PathSeparator) + f
						fileContent, err := os.ReadFile(terragruntFile)
						if err != nil {
							return err
						}
						if strings.Contains(string(fileContent), "include \"root\"") {
							dirs = append(dirs, strings.ReplaceAll(path, workingDir+string(os.PathSeparator), ""))
							return filepath.SkipDir
						}
					}
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

var ErrDiggerConfigConflict = errors.New("more than one digger config file detected, please keep either 'digger.yml' or 'digger.yaml'")

func LoadDiggerConfig(workingDir string) (*DiggerConfig, *DiggerConfigYaml, graph.Graph[string, string], error) {
	config := &DiggerConfig{}
	configYaml, err := LoadDiggerConfigYaml(workingDir)
	if err != nil {
		return nil, nil, nil, err
	}

	config, projectDependencyGraph, err := ConvertDiggerYamlToConfig(configYaml)
	if err != nil {
		return nil, nil, nil, err
	}

	err = ValidateDiggerConfig(config)
	if err != nil {
		return config, configYaml, projectDependencyGraph, err
	}
	return config, configYaml, projectDependencyGraph, nil
}

func LoadDiggerConfigFromString(yamlString string, terraformDir string) (*DiggerConfig, *DiggerConfigYaml, graph.Graph[string, string], error) {
	config := &DiggerConfig{}
	configYaml, err := LoadDiggerConfigYamlFromString(yamlString)
	if err != nil {
		return nil, nil, nil, err
	}

	err = ValidateDiggerConfigYaml(configYaml, "loaded_yaml_string")
	if err != nil {
		return nil, nil, nil, err
	}

	HandleYamlProjectGeneration(configYaml, terraformDir)

	config, projectDependencyGraph, err := ConvertDiggerYamlToConfig(configYaml)
	if err != nil {
		return nil, nil, nil, err
	}

	err = ValidateDiggerConfig(config)
	if err != nil {
		return config, configYaml, projectDependencyGraph, err
	}
	return config, configYaml, projectDependencyGraph, nil
}

func LoadDiggerConfigYamlFromString(yamlString string) (*DiggerConfigYaml, error) {
	configYaml := &DiggerConfigYaml{}
	if err := yaml.Unmarshal([]byte(yamlString), configYaml); err != nil {
		return nil, fmt.Errorf("error parsing yaml: %v", err)
	}

	return configYaml, nil
}

func HandleYamlProjectGeneration(config *DiggerConfigYaml, terraformDir string) {
	if config.GenerateProjectsConfig != nil && config.GenerateProjectsConfig.TerragruntParsingConfig != nil {
		hydrateDiggerConfigYamlWithTerragrunt(config, *config.GenerateProjectsConfig.TerragruntParsingConfig, terraformDir)
	} else if config.GenerateProjectsConfig != nil && config.GenerateProjectsConfig.Terragrunt {
		hydrateDiggerConfigYamlWithTerragrunt(config, TerragruntParsingConfig{}, terraformDir)
	} else if config.GenerateProjectsConfig != nil {
		var dirWalker = &FileSystemTopLevelTerraformDirWalker{}
		dirs, err := dirWalker.GetDirs(terraformDir)

		if err != nil {
			fmt.Printf("Error while walking through directories: %v", err)
		}

		for _, dir := range dirs {
			includePattern := config.GenerateProjectsConfig.Include
			excludePattern := config.GenerateProjectsConfig.Exclude
			if MatchIncludeExcludePatternsToFile(dir, []string{includePattern}, []string{excludePattern}) {
				project := ProjectYaml{Name: filepath.Base(dir), Dir: dir, Workflow: defaultWorkflowName, Workspace: "default"}
				config.Projects = append(config.Projects, &project)
			}
		}
	}
}

func LoadDiggerConfigYaml(workingDir string) (*DiggerConfigYaml, error) {
	configYaml := &DiggerConfigYaml{}
	fileName, err := retrieveConfigFile(workingDir)
	if err != nil {
		if errors.Is(err, ErrDiggerConfigConflict) {
			return nil, fmt.Errorf("error while retrieving config file: %v", err)
		}
	}

	if fileName == "" {
		configYaml, err = AutoDetectDiggerConfig(workingDir)
		if err != nil {
			return nil, fmt.Errorf("failed to auto detect digger config: %v", err)
		}
		marshalledConfig, err := yaml.Marshal(configYaml)
		if err != nil {
			log.Printf("failed to marshal auto detected digger config: %v", err)
		} else {
			log.Printf("Auto detected digger config: \n%v", string(marshalledConfig))
		}
	} else {
		data, err := os.ReadFile(fileName)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %v", fileName, err)
		}

		if err := yaml.Unmarshal(data, configYaml); err != nil {
			return nil, fmt.Errorf("error parsing '%s': %v", fileName, err)
		}
	}

	err = ValidateDiggerConfigYaml(configYaml, fileName)
	if err != nil {
		return configYaml, err
	}

	HandleYamlProjectGeneration(configYaml, workingDir)

	return configYaml, nil
}

func ValidateDiggerConfigYaml(configYaml *DiggerConfigYaml, fileName string) error {
	if (configYaml.Projects == nil || len(configYaml.Projects) == 0) && configYaml.GenerateProjectsConfig == nil {
		return fmt.Errorf("no projects configuration found in '%s'", fileName)
	}
	return nil
}

func ValidateDiggerConfig(config *DiggerConfig) error {
	for _, p := range config.Projects {
		_, ok := config.Workflows[p.Workflow]
		if !ok {
			return fmt.Errorf("failed to find workflow config '%s' for project '%s'", p.Workflow, p.Name)
		}
	}

	for _, w := range config.Workflows {
		for _, s := range w.Plan.Steps {
			if s.Action == "" {
				return fmt.Errorf("plan step's action can't be empty")
			}
		}
	}

	for _, w := range config.Workflows {
		for _, s := range w.Apply.Steps {
			if s.Action == "" {
				return fmt.Errorf("apply step's action can't be empty")
			}
		}
	}
	return nil
}

func hydrateDiggerConfigYamlWithTerragrunt(configYaml *DiggerConfigYaml, parsingConfig TerragruntParsingConfig, workingDir string) {
	root := workingDir
	if parsingConfig.GitRoot != nil {
		root = path.Join(workingDir, *parsingConfig.GitRoot)
	}
	projectExternalChilds := true

	if parsingConfig.CreateHclProjectExternalChilds != nil {
		projectExternalChilds = *parsingConfig.CreateHclProjectExternalChilds
	}

	parallel := true
	if parsingConfig.Parallel != nil {
		parallel = *parsingConfig.Parallel
	}

	ignoreParentTerragrunt := true
	if parsingConfig.IgnoreParentTerragrunt != nil {
		ignoreParentTerragrunt = *parsingConfig.IgnoreParentTerragrunt
	}

	cascadeDependencies := true
	if parsingConfig.CascadeDependencies != nil {
		cascadeDependencies = *parsingConfig.CascadeDependencies
	}

	atlantisConfig, _, err := atlantis.Parse(
		root,
		parsingConfig.ProjectHclFiles,
		projectExternalChilds,
		parsingConfig.AutoMerge,
		parallel,
		parsingConfig.FilterPath,
		parsingConfig.CreateHclProjectChilds,
		ignoreParentTerragrunt,
		parsingConfig.IgnoreDependencyBlocks,
		cascadeDependencies,
		parsingConfig.DefaultWorkflow,
		parsingConfig.DefaultApplyRequirements,
		parsingConfig.AutoPlan,
		parsingConfig.DefaultTerraformVersion,
		parsingConfig.CreateProjectName,
		parsingConfig.CreateWorkspace,
		parsingConfig.PreserveProjects,
		parsingConfig.UseProjectMarkers,
	)
	if err != nil {
		log.Printf("failed to autogenerate config: %v", err)
	}

	configYaml.AutoMerge = &atlantisConfig.AutoMerge
	for _, atlantisProject := range atlantisConfig.Projects {
		configYaml.Projects = append(configYaml.Projects, &ProjectYaml{
			Name:            atlantisProject.Name,
			Dir:             atlantisProject.Dir,
			Workspace:       atlantisProject.Workspace,
			Terragrunt:      true,
			Workflow:        atlantisProject.Workflow,
			IncludePatterns: atlantisProject.Autoplan.WhenModified,
		})
	}
}

func AutoDetectDiggerConfig(workingDir string) (*DiggerConfigYaml, error) {
	configYaml := &DiggerConfigYaml{}
	collectUsageData := true
	configYaml.CollectUsageData = &collectUsageData

	terragruntDirWalker := &FileSystemTerragruntDirWalker{}
	terraformDirWalker := &FileSystemTopLevelTerraformDirWalker{}
	moduleDirWalker := &FileSystemModuleDirWalker{}

	terragruntDirs, err := terragruntDirWalker.GetDirs(workingDir)

	if err != nil {
		return nil, err
	}

	terraformDirs, err := terraformDirWalker.GetDirs(workingDir)
	if err != nil {
		return nil, err
	}

	moduleDirs, err := moduleDirWalker.GetDirs(workingDir)

	var modulePatterns []string
	for _, dir := range moduleDirs {
		modulePatterns = append(modulePatterns, dir+"/**")
	}

	if err != nil {
		return nil, err
	}
	if len(terragruntDirs) > 0 {
		configYaml.GenerateProjectsConfig = &GenerateProjectsConfigYaml{
			Terragrunt: true,
		}
		return configYaml, nil
	} else if len(terraformDirs) > 0 {
		for _, dir := range terraformDirs {
			projectName := dir
			if dir == "./" {
				projectName = "default"
			}
			project := ProjectYaml{Name: projectName, Dir: dir, Workflow: defaultWorkflowName, Workspace: "default", Terragrunt: false, IncludePatterns: modulePatterns}
			configYaml.Projects = append(configYaml.Projects, &project)
		}
		return configYaml, nil
	} else {
		return nil, fmt.Errorf("no terragrunt or terraform project detected in the repository")
	}
}

func (c *DiggerConfig) GetProject(projectName string) *Project {
	for _, project := range c.Projects {
		if projectName == project.Name {
			return &project
		}
	}
	return nil
}

func (c *DiggerConfig) GetProjects(projectName string) []Project {
	if projectName == "" {
		return c.Projects
	}
	project := c.GetProject(projectName)
	if project == nil {
		return nil
	}
	return []Project{*project}
}

func (c *DiggerConfig) GetModifiedProjects(changedFiles []string) []Project {
	var result []Project
	for _, project := range c.Projects {
		for _, changedFile := range changedFiles {
			includePatterns := project.IncludePatterns
			excludePatterns := project.ExcludePatterns
			if !project.Terragrunt {
				includePatterns = append(includePatterns, filepath.Join(project.Dir, "**", "*.tf"))
			} else {
				includePatterns = append(includePatterns, filepath.Join(project.Dir, "*.hcl"))
			}
			// all our patterns are the globale dir pattern + the include patterns specified by user
			if MatchIncludeExcludePatternsToFile(changedFile, includePatterns, excludePatterns) {
				result = append(result, project)
				break
			}
		}
	}
	return result
}

func (c *DiggerConfig) GetDirectory(projectName string) string {
	project := c.GetProject(projectName)
	if project == nil {
		return ""
	}
	return project.Dir
}

func (c *DiggerConfig) GetWorkflow(workflowName string) *Workflow {
	workflows := c.Workflows

	workflow, ok := workflows[workflowName]
	if !ok {
		return nil
	}
	return &workflow

}

type File struct {
	Filename string
}

func isFileExists(path string) bool {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	// file exists make sure it's not a directory
	return !fi.IsDir()
}

func retrieveConfigFile(workingDir string) (string, error) {
	fileName := "digger"
	if workingDir != "" {
		fileName = path.Join(workingDir, fileName)
	}

	// Make sure we don't have more than one digger config file
	ymlCfg := isFileExists(fileName + ".yml")
	yamlCfg := isFileExists(fileName + ".yaml")
	if ymlCfg && yamlCfg {
		return "", ErrDiggerConfigConflict
	}

	// At this point we know there are no duplicates
	// Return the first one that exists
	if ymlCfg {
		return path.Join(workingDir, "digger.yml"), nil
	}
	if yamlCfg {
		return path.Join(workingDir, "digger.yaml"), nil
	}

	// Passing this point means digger config file is
	// missing which is a non-error
	return "", nil
}

func CollectTerraformEnvConfig(envs *TerraformEnvConfig) (map[string]string, map[string]string) {
	stateEnvVars := map[string]string{}
	commandEnvVars := map[string]string{}

	if envs != nil {
		for _, envvar := range envs.State {
			if envvar.Value != "" {
				stateEnvVars[envvar.Name] = envvar.Value
			} else if envvar.ValueFrom != "" {
				stateEnvVars[envvar.Name] = os.Getenv(envvar.ValueFrom)
			}
		}

		for _, envvar := range envs.Commands {
			if envvar.Value != "" {
				commandEnvVars[envvar.Name] = envvar.Value
			} else if envvar.ValueFrom != "" {
				commandEnvVars[envvar.Name] = os.Getenv(envvar.ValueFrom)
			}
		}
	}

	return stateEnvVars, commandEnvVars
}
