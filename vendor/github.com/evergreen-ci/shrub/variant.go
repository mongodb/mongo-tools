package shrub

// Variant represents a single build variant to generate.
type Variant struct {
	BuildName        string                  `json:"name,omitempty" yaml:"name,omitempty"`
	BuildDisplayName string                  `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Tags             []string                `json:"tags,omitempty" yaml:"tags,omitempty"`
	BatchTimeSecs    int                     `json:"batchtime,omitempty" yaml:"batchtime,omitempty"`
	CronBatchTime    string                  `json:"cron,omitempty" yaml:"cron,omitempty"`
	Stepback         *bool                   `json:"stepback,omitempty" yaml:"stepback,omitempty"`
	TaskSpecs        []TaskSpec              `json:"tasks,omitmepty" yaml:"tasks,omitempty"`
	DistroRunOn      []string                `json:"run_on,omitempty" yaml:"run_on,omitempty"`
	Expansions       map[string]interface{}  `json:"expansions,omitempty" yaml:"expansions,omitempty"`
	DisplayTaskSpecs []DisplayTaskDefinition `json:"display_tasks,omitempty" yaml:"display_tasks,omitempty"`
	Modules          []string                `json:"modules,omitempty" yaml:"modules,omitempty"`
	DependsOn        []TaskDependency        `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	// If Activate is set to false, then we don't initially activate the build variant.
	Activate          *bool    `json:"activate,omitempty" yaml:"activate,omitempty"`
	Disable           *bool    `json:"disable,omitempty" yaml:"disable,omitempty"`
	Patchable         *bool    `json:"patchable,omitempty" yaml:"patchable,omitempty"`
	PatchOnly         *bool    `json:"patch_only,omitempty" yaml:"patch_only,omitempty"`
	AllowForGitTag    *bool    `json:"allow_for_git_tag,omitempty" yaml:"allow_for_git_tag,omitempty"`
	GitTagOnly        *bool    `json:"git_tag_only,omitempty" yaml:"git_tag_only,omitempty"`
	AllowedRequesters []string `json:"allowed_requesters,omitempty" yaml:"allowed_requesters,omitempty"`
}

type DisplayTaskDefinition struct {
	Name       string   `json:"name" yaml:"name"`
	Components []string `json:"execution_tasks" yaml:"execution_tasks"`
}

type TaskSpec struct {
	Name     string `json:"name" yaml:"name"`
	Stepback bool   `json:"stepback,omitempty" yaml:"stepback,omitempty"`
	// Distro is deprecated in favor of RunOn.
	Distro            []string         `json:"distros,omitempty" yaml:"distro,omitempty"`
	RunOn             []string         `json:"run_on,omitempty" yaml:"run_on,omitempty"`
	DependsOn         []TaskDependency `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Priority          int              `json:"priority,omitempty" yaml:"priority,omitempty"`
	ExecTimeoutSecs   int              `json:"exec_timeout_secs,omitempty" yaml:"exec_timeout_secs,omitempty"`
	Batchtime         int              `json:"batchtime,omitempty" yaml:"batchtime,omitempty"`
	CronBatchtime     string           `json:"cron_batchtime,omitempty" yaml:"cron_batchtime,omitempty"`
	Activate          *bool            `json:"activate,omitempty" yaml:"activate,omitempty"`
	Disable           *bool            `json:"disable,omitempty" yaml:"disable,omitempty"`
	Patchable         *bool            `json:"patchable,omitempty" yaml:"patchable,omitempty"`
	PatchOnly         *bool            `json:"patch_only,omitempty" yaml:"patch_only,omitempty"`
	AllowForGitTag    *bool            `json:"allow_for_git_tag,omitempty" yaml:"allow_for_git_tag,omitempty"`
	GitTagOnly        *bool            `json:"git_tag_only,omitempty" yaml:"git_tag_only,omitempty"`
	AllowedRequesters []string         `json:"allowed_requesters,omitempty" yaml:"allowed_requesters,omitempty"`
	TaskGroup         *TaskGroup       `json:"task_group,omitempty" yaml:"task_group,omitempty"`
	CreateCheckRun    *CheckRun        `json:"create_check_run,omitempty" yaml:"create_check_run,omitempty"`
}

type CheckRun struct {
	PathToOutputs string `json:"path_to_outputs,omitempty" yaml:"path_to_outputs,omitempty"`
}

func (cr *CheckRun) SetPathToOutputs(path string) *CheckRun {
	cr.PathToOutputs = path
	return cr
}

func (ts *TaskSpec) SetName(name string) *TaskSpec { ts.Name = name; return ts }
func (ts *TaskSpec) SetStepback(shouldStepback bool) *TaskSpec {
	ts.Stepback = shouldStepback
	return ts
}

// SetDistros is deprecated in favor of RunOn.
func (ts *TaskSpec) SetDistros(distros []string) *TaskSpec { ts.Distro = distros; return ts }

func (ts *TaskSpec) SetRunOn(distros ...string) *TaskSpec {
	ts.RunOn = distros
	return ts
}

func (ts *TaskSpec) SetDependsOn(deps ...TaskDependency) *TaskSpec {
	ts.DependsOn = deps
	return ts
}

func (ts *TaskSpec) SetPriority(priority int) *TaskSpec {
	ts.Priority = priority
	return ts
}

func (ts *TaskSpec) SetExecTimeoutSecs(timeoutSecs int) *TaskSpec {
	ts.ExecTimeoutSecs = timeoutSecs
	return ts
}

func (ts *TaskSpec) SetBatchtime(batchtime int) *TaskSpec {
	ts.Batchtime = batchtime
	return ts
}

func (ts *TaskSpec) SetCronBatchtime(cron string) *TaskSpec {
	ts.CronBatchtime = cron
	return ts
}

func (ts *TaskSpec) SetTaskGroup(tg TaskGroup) *TaskSpec {
	ts.TaskGroup = &tg
	return ts
}

func (ts *TaskSpec) SetCreateCheckrun(checkRun CheckRun) *TaskSpec {
	ts.CreateCheckRun = &checkRun
	return ts
}

func (ts *TaskSpec) SetActivate(shouldActivate *bool) *TaskSpec {
	ts.Activate = shouldActivate
	return ts
}

func (ts *TaskSpec) SetDisable(disable *bool) *TaskSpec {
	ts.Disable = disable
	return ts
}

func (ts *TaskSpec) SetPatchable(patchable *bool) *TaskSpec {
	ts.Patchable = patchable
	return ts
}

func (ts *TaskSpec) SetPatchOnly(patchOnly *bool) *TaskSpec {
	ts.PatchOnly = patchOnly
	return ts
}

func (ts *TaskSpec) SetAllowForGitTag(allowForGitTag *bool) *TaskSpec {
	ts.AllowForGitTag = allowForGitTag
	return ts
}

func (ts *TaskSpec) SetGitTagOnly(gitTagOnly *bool) *TaskSpec {
	ts.GitTagOnly = gitTagOnly
	return ts
}

func (ts *TaskSpec) AllowedRequester(requesters ...string) *TaskSpec {
	ts.AllowedRequesters = append(ts.AllowedRequesters, requesters...)
	return ts
}

func (v *Variant) Name(id string) *Variant { v.BuildName = id; return v }
func (v *Variant) SetTags(tags ...string) *Variant {
	v.Tags = tags
	return v
}
func (v *Variant) BatchTime(batchTimeSecs int) *Variant       { v.BatchTimeSecs = batchTimeSecs; return v }
func (v *Variant) SetCronBatchTime(batchTime string) *Variant { v.CronBatchTime = batchTime; return v }
func (v *Variant) SetStepback(stepback *bool) *Variant        { v.Stepback = stepback; return v }
func (v *Variant) SetActivate(activate *bool) *Variant        { v.Activate = activate; return v }
func (v *Variant) SetDisable(disable *bool) *Variant          { v.Disable = disable; return v }
func (v *Variant) SetPatchable(patchable *bool) *Variant      { v.Patchable = patchable; return v }
func (v *Variant) SetPatchOnly(patchOnly *bool) *Variant      { v.PatchOnly = patchOnly; return v }
func (v *Variant) SetAllowForGitTag(allow *bool) *Variant     { v.AllowForGitTag = allow; return v }
func (v *Variant) SetGitTagOnly(gitTagOnly *bool) *Variant    { v.GitTagOnly = gitTagOnly; return v }
func (v *Variant) AllowedRequester(requesters ...string) *Variant {
	v.AllowedRequesters = append(v.AllowedRequesters, requesters...)
	return v
}

func (v *Variant) DisplayName(id string) *Variant  { v.BuildDisplayName = id; return v }
func (v *Variant) RunOn(distro string) *Variant    { v.DistroRunOn = []string{distro}; return v }
func (v *Variant) TaskSpec(spec TaskSpec) *Variant { v.TaskSpecs = append(v.TaskSpecs, spec); return v }
func (v *Variant) Module(module string) *Variant   { v.Modules = append(v.Modules, module); return v }
func (v *Variant) SetDependsOn(deps ...TaskDependency) *Variant {
	v.DependsOn = deps
	return v
}
func (v *Variant) SetExpansions(m map[string]interface{}) *Variant { v.Expansions = m; return v }

func (v *Variant) Expansion(k string, val interface{}) *Variant {
	if v.Expansions == nil {
		v.Expansions = make(map[string]interface{})
	}

	v.Expansions[k] = val

	return v
}

func (v *Variant) AddTasks(name ...string) *Variant {
	for _, n := range name {
		if n == "" {
			continue
		}

		v.TaskSpecs = append(v.TaskSpecs, TaskSpec{
			Name: n,
		})
	}
	return v
}

func (v *Variant) DisplayTasks(def ...DisplayTaskDefinition) *Variant {
	v.DisplayTaskSpecs = append(v.DisplayTaskSpecs, def...)
	return v
}
