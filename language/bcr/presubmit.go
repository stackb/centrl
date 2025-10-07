package bcr

import (
	"fmt"
	"os"

	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"gopkg.in/yaml.v3"
)

// presubmitYAML represents the structure of a presubmit.yml file
type presubmitYAML struct {
	BcrTestModule *bcrTestModuleYAML   `yaml:"bcr_test_module,omitempty"`
	Matrix        *presubmitMatrixYAML `yaml:"matrix,omitempty"`
	Tasks         map[string]*taskYAML `yaml:"tasks,omitempty"`
}

// bcrTestModuleYAML represents the bcr_test_module section
type bcrTestModuleYAML struct {
	ModulePath string               `yaml:"module_path"`
	Matrix     *presubmitMatrixYAML `yaml:"matrix,omitempty"`
	Tasks      map[string]*taskYAML `yaml:"tasks,omitempty"`
}

// presubmitMatrixYAML represents the matrix configuration
type presubmitMatrixYAML struct {
	Platform []string `yaml:"platform,omitempty"`
	Bazel    []string `yaml:"bazel,omitempty"`
}

// taskYAML represents a task configuration
type taskYAML struct {
	Name         string   `yaml:"name,omitempty"`
	Platform     string   `yaml:"platform,omitempty"`
	Bazel        string   `yaml:"bazel,omitempty"`
	BuildFlags   []string `yaml:"build_flags,omitempty"`
	TestFlags    []string `yaml:"test_flags,omitempty"`
	BuildTargets []string `yaml:"build_targets,omitempty"`
	TestTargets  []string `yaml:"test_targets,omitempty"`
}

// readPresubmitYaml reads and parses a presubmit.yml file
func readPresubmitYaml(filename string) (*bzpb.Presubmit, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading presubmit.yml file: %v", err)
	}

	var yamlData presubmitYAML
	if err := yaml.Unmarshal(data, &yamlData); err != nil {
		return nil, fmt.Errorf("parsing presubmit.yml file: %v", err)
	}

	return convertPresubmitYAMLToProto(&yamlData), nil
}

// convertPresubmitYAMLToProto converts the YAML structure to protobuf
func convertPresubmitYAMLToProto(y *presubmitYAML) *bzpb.Presubmit {
	presubmit := &bzpb.Presubmit{}

	if y.BcrTestModule != nil {
		presubmit.BcrTestModule = &bzpb.Presubmit_BcrTestModule{
			ModulePath: y.BcrTestModule.ModulePath,
		}

		if y.BcrTestModule.Matrix != nil {
			presubmit.BcrTestModule.Matrix = convertMatrixYAMLToProto(y.BcrTestModule.Matrix)
		}

		if len(y.BcrTestModule.Tasks) > 0 {
			presubmit.BcrTestModule.Tasks = make(map[string]*bzpb.Presubmit_PresubmitTask)
			for name, task := range y.BcrTestModule.Tasks {
				presubmit.BcrTestModule.Tasks[name] = convertTaskYAMLToProto(task)
			}
		}
	}

	if y.Matrix != nil {
		presubmit.Matrix = convertMatrixYAMLToProto(y.Matrix)
	}

	if len(y.Tasks) > 0 {
		presubmit.Tasks = make(map[string]*bzpb.Presubmit_PresubmitTask)
		for name, task := range y.Tasks {
			presubmit.Tasks[name] = convertTaskYAMLToProto(task)
		}
	}

	return presubmit
}

// convertMatrixYAMLToProto converts matrix YAML to protobuf
func convertMatrixYAMLToProto(m *presubmitMatrixYAML) *bzpb.Presubmit_PresubmitMatrix {
	return &bzpb.Presubmit_PresubmitMatrix{
		Platform: m.Platform,
		Bazel:    m.Bazel,
	}
}

// convertTaskYAMLToProto converts task YAML to protobuf
func convertTaskYAMLToProto(t *taskYAML) *bzpb.Presubmit_PresubmitTask {
	return &bzpb.Presubmit_PresubmitTask{
		Name:         t.Name,
		Platform:     t.Platform,
		Bazel:        t.Bazel,
		BuildFlags:   t.BuildFlags,
		TestFlags:    t.TestFlags,
		BuildTargets: t.BuildTargets,
		TestTargets:  t.TestTargets,
	}
}
