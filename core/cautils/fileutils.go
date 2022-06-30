package cautils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/armosec/k8s-interface/workloadinterface"

	"github.com/armosec/kubescape/v2/core/cautils/logger"
	"github.com/armosec/opa-utils/objectsenvelopes"
	"github.com/armosec/opa-utils/objectsenvelopes/localworkload"
	"gopkg.in/yaml.v2"
)

var (
	YAML_PREFIX = []string{"yaml", "yml"}
	JSON_PREFIX = []string{"json"}
)

type FileFormat string

const (
	YAML_FILE_FORMAT FileFormat = "yaml"
	JSON_FILE_FORMAT FileFormat = "json"
)

func LoadResourcesFromFiles(input string) (map[string][]workloadinterface.IMetadata, error) {
	files, errs := listFiles(input)
	if len(errs) > 0 {
		logger.L().Error(fmt.Sprintf("%v", errs))
	}
	if len(files) == 0 {
		return nil, nil
	}

	workloads, errs := loadFiles(files)
	if len(errs) > 0 {
		logger.L().Error(fmt.Sprintf("%v", errs))
	}

	return workloads, nil
}

func loadFiles(filePaths []string) (map[string][]workloadinterface.IMetadata, []error) {
	workloads := make(map[string][]workloadinterface.IMetadata, 0)
	errs := []error{}
	for i := range filePaths {
		f, err := loadFile(filePaths[i])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(f) == 0 {
			continue // empty file
		}
		w, e := ReadFile(f, GetFileFormat(filePaths[i]))
		errs = append(errs, e...)
		if w != nil {
			path := filePaths[i]
			if _, ok := workloads[path]; !ok {
				workloads[path] = []workloadinterface.IMetadata{}
			}
			wSlice := workloads[path]
			for j := range w {
				lw := localworkload.NewLocalWorkload(w[j].GetObject())
				lw.SetPath(path)
				wSlice = append(wSlice, lw)
			}
			workloads[path] = wSlice
		}
	}
	return workloads, errs
}

func loadFile(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}
func ReadFile(fileContent []byte, fileFormat FileFormat) ([]workloadinterface.IMetadata, []error) {

	switch fileFormat {
	case YAML_FILE_FORMAT:
		return readYamlFile(fileContent)
	case JSON_FILE_FORMAT:
		return readJsonFile(fileContent)
	default:
		return nil, nil // []error{fmt.Errorf("file extension %s not supported", fileFormat)}
	}
}

// listFiles returns the list of absolute paths, full file path and list of errors. The list of abs paths and full path have the same length
func listFiles(pattern string) ([]string, []error) {

	var files []string
	errs := []error{}

	if !filepath.IsAbs(pattern) {
		o, _ := os.Getwd()
		pattern = filepath.Join(o, pattern)
	}

	if IsFile(pattern) {
		files = append(files, pattern)
		return files, errs
	}

	root, shouldMatch := filepath.Split(pattern)

	if IsDir(pattern) {
		root = pattern
		shouldMatch = "*"
	}
	if shouldMatch == "" {
		shouldMatch = "*"
	}

	f, err := glob(root, shouldMatch)
	if err != nil {
		errs = append(errs, err)
	} else {
		files = append(files, f...)
	}

	return files, errs
}

func readYamlFile(yamlFile []byte) ([]workloadinterface.IMetadata, []error) {
	errs := []error{}

	r := bytes.NewReader(yamlFile)
	dec := yaml.NewDecoder(r)
	yamlObjs := []workloadinterface.IMetadata{}

	var t interface{}
	for dec.Decode(&t) == nil {
		j := convertYamlToJson(t)
		if j == nil {
			continue
		}
		if obj, ok := j.(map[string]interface{}); ok {
			if o := objectsenvelopes.NewObject(obj); o != nil {
				if o.GetKind() == "List" {
					yamlObjs = append(yamlObjs, handleListObject(o)...)
				} else {
					yamlObjs = append(yamlObjs, o)
				}
			}
		}
	}

	return yamlObjs, errs
}

func readJsonFile(jsonFile []byte) ([]workloadinterface.IMetadata, []error) {
	workloads := []workloadinterface.IMetadata{}
	var jsonObj interface{}
	if err := json.Unmarshal(jsonFile, &jsonObj); err != nil {
		return workloads, []error{err}
	}

	convertJsonToWorkload(jsonObj, &workloads)

	return workloads, nil
}
func convertJsonToWorkload(jsonObj interface{}, workloads *[]workloadinterface.IMetadata) {

	switch x := jsonObj.(type) {
	case map[string]interface{}:
		if o := objectsenvelopes.NewObject(x); o != nil {
			(*workloads) = append(*workloads, o)
		}
	case []interface{}:
		for i := range x {
			convertJsonToWorkload(x[i], workloads)
		}
	}
}
func convertYamlToJson(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			if s, ok := k.(string); ok {
				m2[s] = convertYamlToJson(v)
			}
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convertYamlToJson(v)
		}
	}
	return i
}

func IsYaml(filePath string) bool {
	return StringInSlice(YAML_PREFIX, strings.ReplaceAll(filepath.Ext(filePath), ".", "")) != ValueNotFound
}

func IsJson(filePath string) bool {
	return StringInSlice(JSON_PREFIX, strings.ReplaceAll(filepath.Ext(filePath), ".", "")) != ValueNotFound
}

func glob(root, pattern string) ([]string, error) {
	var matches []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fileFormat := GetFileFormat(path)
		if !(fileFormat == JSON_FILE_FORMAT || fileFormat == YAML_FILE_FORMAT) {
			return nil
		}
		if matched, err := filepath.Match(pattern, filepath.Base(path)); err != nil {
			return err
		} else if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// IsFile checks if a given path is a file
func IsFile(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		if fi.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// IsDir checks if a given path is a directory
func IsDir(name string) bool {
	if info, err := os.Stat(name); err == nil {
		if info.IsDir() {
			return true
		}
	}
	return false
}

func GetFileFormat(filePath string) FileFormat {
	if IsYaml(filePath) {
		return YAML_FILE_FORMAT
	} else if IsJson(filePath) {
		return JSON_FILE_FORMAT
	} else {
		return FileFormat(filePath)
	}
}

// handleListObject handle a List manifest
func handleListObject(obj workloadinterface.IMetadata) []workloadinterface.IMetadata {
	yamlObjs := []workloadinterface.IMetadata{}
	if i, ok := workloadinterface.InspectMap(obj.GetObject(), "items"); ok && i != nil {
		if items, ok := i.([]interface{}); ok && items != nil {
			for item := range items {
				if m, ok := items[item].(map[string]interface{}); ok && m != nil {
					if o := objectsenvelopes.NewObject(m); o != nil {
						yamlObjs = append(yamlObjs, o)
					}
				}
			}
		}
	}
	return yamlObjs
}