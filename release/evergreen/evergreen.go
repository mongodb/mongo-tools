package evergreen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	EVG_API_BASE_URI    = "https://evergreen.mongodb.com/rest/v2"
	EVG_API_HEADER_USER = "Api-User"
	EVG_API_HEADER_KEY  = "Api-Key"

	EVG_USER_ENV_VAR = "EVG_USER"
	EVG_KEY_ENV_VAR  = "EVG_KEY"
)

type Task struct {
	TaskID      string `json:"task_id"`
	BuildID     string `json:"build_id"`
	Variant     string `json:"build_variant"`
	Status      string `json:"status"`
	DisplayName string `json:"display_name"`
}

type BuildDetail struct {
	BuildVariant string `json:"build_variant"`
	BuildID      string `json:"build_id"`
}

// EvgVersion is a container for the /versions/<id> endpoint response.
// We only decode build_variants_status since that's all we care about.
type EvgVersion struct {
	BuildVariantStatus []BuildDetail `json:"build_variants_status"`
}

// IsPatch indicates whether the task is part of a patchbuild (as
// opposed to a task from the waterfall).
func (t Task) IsPatch() bool {
	return strings.Contains(t.TaskID, "patch")
}

type Artifact struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func newAPIError(method, path string, res *http.Response) error {
	bodyBytes, _ := json.Marshal(res.Body)
	body := string(bodyBytes)
	return APIError{
		Method: method,
		Path:   path,
		Status: res.StatusCode,
		Body:   body,
	}
}

func (e APIError) Error() string {
	return fmt.Sprintf(
		"%s %s failed with code %d: %s",
		e.Method, e.Path, e.Status, e.Body,
	)
}

func (e APIError) String() string {
	return e.Error()
}

func GetArtifactsForTask(id string) ([]Artifact, error) {
	res, err := get("/tasks/" + id)
	if err != nil {
		return nil, err
	}

	task := struct {
		Artifacts []Artifact `json:"artifacts"`
		Status    string     `json:"status"`
	}{}
	bodyDecoder := json.NewDecoder(res.Body)
	err = bodyDecoder.Decode(&task)
	if err != nil {
		return nil, err
	}

	if task.Status != "success" {
		return nil, fmt.Errorf("task state is '%s', not 'success'", task.Status)
	}

	return task.Artifacts, nil
}

// GetPackageTaskForVersion gets the package tasks associated with a particular variant and version.
// This is used to get the package task from the mongo-release project, which is then used to
// download the jstestshell.
func GetPackageTaskForVersion(variant, version string) (string, error) {
	res, err := get("/versions/" + version)
	if err != nil {
		return "", err
	}

	var evgVersion EvgVersion
	bodyDecoder := json.NewDecoder(res.Body)
	err = bodyDecoder.Decode(&evgVersion)
	if err != nil {
		return "", err
	}

	buildID := ""

	for _, buildDetail := range evgVersion.BuildVariantStatus {
		if buildDetail.BuildVariant == variant {
			buildID = buildDetail.BuildID
			break
		}
	}

	tasks, err := GetTasksForBuild(buildID)
	if err != nil {
		return "", err
	}

	for _, t := range tasks {
		if t.DisplayName == "package" {
			return t.TaskID, nil
		}
	}

	return "", nil
}

// GetTasksForVersion gets all the evergreen tasks associated with a version.
func GetTasksForVersion(version string) ([]Task, error) {
	res, err := get("/versions/" + version)
	if err != nil {
		return nil, err
	}

	var evgVersion EvgVersion
	bodyDecoder := json.NewDecoder(res.Body)
	err = bodyDecoder.Decode(&evgVersion)
	if err != nil {
		return nil, err
	}

	tasks := []Task{}
	for _, buildDetail := range evgVersion.BuildVariantStatus {
		buildTasks, err := GetTasksForBuild(buildDetail.BuildID)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, buildTasks...)
	}
	return tasks, nil
}

// GetTasksForBuild gets all the evergreen tasks associated with a build.
func GetTasksForBuild(build string) ([]Task, error) {
	res, err := get("/builds/" + build + "/tasks?limit=100000")
	if err != nil {
		return nil, err
	}

	tasks := []Task{}
	bodyDecoder := json.NewDecoder(res.Body)
	err = bodyDecoder.Decode(&tasks)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func get(relPath string) (*http.Response, error) {
	uri, err := url.Parse(EVG_API_BASE_URI + relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse evg uri: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, uri.String(), nil)
	if err != nil {
		return nil, err
	}

	user, key, err := getAuthInfo()
	if err != nil {
		return nil, err
	}
	req.Header.Add(EVG_API_HEADER_USER, user)
	req.Header.Add(EVG_API_HEADER_KEY, key)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, newAPIError(req.Method, relPath, res)
	}
	return res, nil
}

func getAuthInfo() (string, string, error) {
	user, key, err := getAuthInfoFromConfig()
	if err != nil {
		return "", "", err
	}
	user, key = overrideAuthInfoFromEnv(user, key)
	if user == "" {
		return "", "", fmt.Errorf("could not obtain evergreen username")
	}
	if key == "" {
		return "", "", fmt.Errorf("could not obtain evergreen key")
	}
	return user, key, nil
}

func getAuthInfoFromConfig() (string, string, error) {
	// For now, we can't read an evg config, so return no info.
	return "", "", nil
}

func overrideAuthInfoFromEnv(user, key string) (string, string) {
	envUser := os.Getenv(EVG_USER_ENV_VAR)
	if envUser != "" {
		user = envUser
	}

	envKey := os.Getenv(EVG_KEY_ENV_VAR)
	if envKey != "" {
		key = envKey
	}

	return user, key
}
