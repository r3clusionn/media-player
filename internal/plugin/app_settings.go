package plugin

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	"seanime/internal/database/models"
	"seanime/internal/extension"
	"seanime/internal/extension_repo/prompt"
	"seanime/internal/goja/goja_bindings"
	gojautil "seanime/internal/util/goja"

	"github.com/dop251/goja"
	"github.com/goccy/go-json"
	"github.com/rs/zerolog"
)

func (a *AppContextImpl) bindSettingsObj(vm *goja.Runtime, ext *extension.Extension, scheduler *gojautil.Scheduler) *goja.Object {
	settingsObj := vm.NewObject()
	cache := newPromptCache()

	_ = settingsObj.Set("get", func(call goja.FunctionCall) goja.Value {
		path := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			path = call.Argument(0).String()
		}

		hasFallback := false
		var fallback interface{}
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			hasFallback = true
			fallback = call.Argument(1).Export()
		}

		detail := "All settings"
		path = strings.TrimSpace(path)
		message := "Allow \"" + ext.Name + "\" to view your app settings?"
		if path != "" {
			detail = "Setting: \"" + path + "\""
			message = "Allow \"" + ext.Name + "\" to view \"" + path + "\"?"
		}

		return a.settingsAction(vm, scheduler, ext, prompt.Options{
			Kind:     "settings",
			Action:   "view \"" + detail + "\"",
			Resource: detail,
			Message:  message,
			Details:  []string{path},
			Cache:    cache,
			CacheKey: settingsCacheKey("view", path),
		}, func() (interface{}, error) {
			settings, err := a.getSettings()
			if err != nil {
				return nil, err
			}
			if path == "" {
				return settings, nil
			}

			base, err := toMap(settings)
			if err != nil {
				return nil, err
			}
			value, found := getPath(base, path)
			if !found && hasFallback {
				return fallback, nil
			}
			return value, nil
		})
	})

	_ = settingsObj.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return rejectNow(vm, errors.New("settings value is required"))
		}

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			path := call.Argument(0).String()
			value := call.Argument(1).Export()
			if strings.TrimSpace(path) == "" {
				return rejectNow(vm, errors.New("settings path is empty"))
			}

			return a.settingsAction(vm, scheduler, ext, prompt.Options{
				Kind:       "settings",
				Action:     "edit \"" + path + "\"",
				Resource:   "Setting: \"" + path + "\"",
				Message:    "Allow \"" + ext.Name + "\" to edit \"" + path + "\"?",
				Details:    []string{path},
				AllowLabel: "Allow",
				DenyLabel:  "Don't Allow",
				Cache:      cache,
				CacheKey:   settingsCacheKey("edit", path),
			}, func() (interface{}, error) {
				settings, err := a.getSettings()
				if err != nil {
					return nil, err
				}

				base, err := toMap(settings)
				if err != nil {
					return nil, err
				}
				if err := setPath(base, path, value); err != nil {
					return nil, err
				}
				next, err := mapToSettings(base)
				if err != nil {
					return nil, err
				}
				return a.saveSettings(next)
			})
		}

		var next models.Settings
		if err := decodeValue(call.Argument(0), &next); err != nil {
			return rejectNow(vm, err)
		}

		details := []string{"all settings"}
		if current, err := a.getSettings(); err == nil {
			details = diffSettingsPaths(current, &next)
			if len(details) == 0 {
				details = []string{"no setting changes"}
			}
		}

		return a.settingsAction(vm, scheduler, ext, prompt.Options{
			Kind:     "settings",
			Action:   "edit app settings",
			Resource: "App settings",
			Message:  "Allow \"" + ext.Name + "\" to edit these settings?",
			Details:  details,
			Cache:    cache,
			CacheKey: settingsCacheKey("edit", details...),
		}, func() (interface{}, error) {
			return a.saveSettings(&next)
		})
	})

	_ = settingsObj.Set("patch", func(patch map[string]interface{}) goja.Value {
		details := settingPaths(patch)
		if len(details) == 0 {
			details = []string{"app settings"}
		}

		return a.settingsAction(vm, scheduler, ext, prompt.Options{
			Kind:     "settings",
			Action:   "edit app settings",
			Resource: "App settings",
			Message:  "Allow \"" + ext.Name + "\" to edit these settings?",
			Details:  details,
			Cache:    cache,
			CacheKey: settingsCacheKey("edit", details...),
		}, func() (interface{}, error) {
			settings, err := a.getSettings()
			if err != nil {
				return nil, err
			}

			next, err := patchSettings(settings, patch)
			if err != nil {
				return nil, err
			}
			return a.saveSettings(next)
		})
	})

	return settingsObj
}

func settingsCacheKey(action string, parts ...string) string {
	key := strings.Join(parts, "|")
	if key == "" {
		key = "all"
	}
	return promptKey("settings", action, key)
}

func (a *AppContextImpl) BindAppSettingsToContextObj(vm *goja.Runtime, obj *goja.Object, logger *zerolog.Logger, ext *extension.Extension, scheduler *gojautil.Scheduler) {
	_ = logger
	_ = obj.Set("appSettings", a.bindSettingsObj(vm, ext, scheduler))
}

func (a *AppContextImpl) settingsAction(vm *goja.Runtime, scheduler *gojautil.Scheduler, ext *extension.Extension, opts prompt.Options, run func() (interface{}, error)) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		ret, err := interface{}(nil), a.ask(ext, opts)
		if err == nil {
			ret, err = run()
		}

		scheduler.ScheduleAsync(func() error {
			if err != nil {
				reject(goja_bindings.NewErrorString(vm, err.Error()))
				return nil
			}
			resolve(vm.ToValue(ret))
			return nil
		})
	}()

	return vm.ToValue(promise)
}

func (a *AppContextImpl) saveSettings(settings *models.Settings) (*models.Settings, error) {
	if settings == nil {
		return nil, errors.New("settings is nil")
	}

	database, ok := a.database.Get()
	if !ok {
		return nil, errors.New("database not set")
	}

	settings.BaseModel = models.BaseModel{ID: 1, UpdatedAt: time.Now()}
	saved, err := database.UpsertSettings(settings)
	if err != nil {
		return nil, err
	}
	if a.settings.OnSaved != nil {
		a.settings.OnSaved(saved)
	}
	return saved, nil
}

func (a *AppContextImpl) getSettings() (*models.Settings, error) {
	database, ok := a.database.Get()
	if !ok {
		return nil, errors.New("database not set")
	}
	return database.GetSettings()
}

func patchSettings(settings *models.Settings, patch map[string]interface{}) (*models.Settings, error) {
	base, err := toMap(settings)
	if err != nil {
		return nil, err
	}
	merge(base, patch)
	return mapToSettings(base)
}

func toMap(in interface{}) (map[string]interface{}, error) {
	bytes, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	var ret map[string]interface{}
	if err := json.Unmarshal(bytes, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func merge(dst map[string]interface{}, src map[string]interface{}) {
	for key, value := range src {
		srcMap, srcOk := value.(map[string]interface{})
		dstMap, dstOk := dst[key].(map[string]interface{})
		if srcOk && dstOk {
			merge(dstMap, srcMap)
			continue
		}
		dst[key] = value
	}
}

func getPath(settings map[string]interface{}, path string) (interface{}, bool) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return settings, true
	}

	var curr interface{} = settings
	for _, part := range parts {
		currMap, ok := curr.(map[string]interface{})
		if !ok {
			return nil, false
		}
		curr, ok = currMap[part]
		if !ok {
			return nil, false
		}
	}

	return curr, true
}

func setPath(settings map[string]interface{}, path string, value interface{}) error {
	parts := splitPath(path)
	if len(parts) == 0 {
		return errors.New("settings path is empty")
	}

	curr := settings
	for _, part := range parts[:len(parts)-1] {
		next, ok := curr[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			curr[part] = next
		}
		curr = next
	}
	curr[parts[len(parts)-1]] = value
	return nil
}

func mapToSettings(in map[string]interface{}) (*models.Settings, error) {
	bytes, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	var ret models.Settings
	if err := json.Unmarshal(bytes, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

func decodeValue(value goja.Value, out interface{}) error {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return errors.New("value is empty")
	}

	bytes, err := json.Marshal(value.Export())
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, out)
}

func rejectNow(vm *goja.Runtime, err error) goja.Value {
	promise, _, reject := vm.NewPromise()
	reject(goja_bindings.NewErrorString(vm, err.Error()))
	return vm.ToValue(promise)
}

func settingPaths(patch map[string]interface{}) []string {
	ret := make([]string, 0)
	collectSettingPaths("", patch, &ret)
	sort.Strings(ret)
	return ret
}

func collectSettingPaths(prefix string, in map[string]interface{}, ret *[]string) {
	for key, value := range in {
		if prefix == "" && isSettingMetaKey(key) {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		valueMap, ok := value.(map[string]interface{})
		if ok && len(valueMap) > 0 {
			collectSettingPaths(path, valueMap, ret)
			continue
		}

		*ret = append(*ret, path)
	}
}

func diffSettingsPaths(prev *models.Settings, next *models.Settings) []string {
	prevMap, err := toMap(prev)
	if err != nil {
		return []string{"all settings"}
	}
	nextMap, err := toMap(next)
	if err != nil {
		return []string{"all settings"}
	}

	ret := make([]string, 0)
	diffMapPaths("", prevMap, nextMap, &ret)
	sort.Strings(ret)
	return ret
}

func diffMapPaths(prefix string, prev map[string]interface{}, next map[string]interface{}, ret *[]string) {
	seen := make(map[string]struct{}, len(prev)+len(next))
	for key := range prev {
		seen[key] = struct{}{}
	}
	for key := range next {
		seen[key] = struct{}{}
	}

	for key := range seen {
		if prefix == "" && isSettingMetaKey(key) {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		prevVal, prevFound := prev[key]
		nextVal, nextFound := next[key]
		prevMap, prevIsMap := prevVal.(map[string]interface{})
		nextMap, nextIsMap := nextVal.(map[string]interface{})
		if prevFound && nextFound && prevIsMap && nextIsMap {
			diffMapPaths(path, prevMap, nextMap, ret)
			continue
		}

		if !prevFound || !nextFound || !reflect.DeepEqual(prevVal, nextVal) {
			*ret = append(*ret, path)
		}
	}
}

func isSettingMetaKey(key string) bool {
	switch key {
	case "id", "createdAt", "updatedAt":
		return true
	default:
		return false
	}
}

func splitPath(path string) []string {
	parts := strings.Split(path, ".")
	ret := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			ret = append(ret, part)
		}
	}
	return ret
}
