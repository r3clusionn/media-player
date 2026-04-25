package plugin

import (
	"seanime/internal/database/db_bridge"
	"seanime/internal/extension"
	"seanime/internal/goja/goja_bindings"
	"seanime/internal/library/anime"
	gojautil "seanime/internal/util/goja"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"
)

func (a *AppContextImpl) BindAutoSelectToContextObj(vm *goja.Runtime, obj *goja.Object, _ *zerolog.Logger, _ *extension.Extension, _ *gojautil.Scheduler) {
	autoSelectObj := vm.NewObject()

	_ = autoSelectObj.Set("getProfile", func() goja.Value {
		database, ok := a.database.Get()
		if !ok {
			goja_bindings.PanicThrowErrorString(vm, "database not set")
		}

		profile, err := db_bridge.GetAutoSelectProfile(database)
		if err != nil || profile == nil {
			return goja.Undefined()
		}

		return vm.ToValue(profile)
	})

	_ = autoSelectObj.Set("saveProfile", func(profile anime.AutoSelectProfile) goja.Value {
		database, ok := a.database.Get()
		if !ok {
			goja_bindings.PanicThrowErrorString(vm, "database not set")
		}

		if err := db_bridge.SaveAutoSelectProfile(database, &profile); err != nil {
			goja_bindings.PanicThrowError(vm, err)
		}

		saved, err := db_bridge.GetAutoSelectProfile(database)
		if err != nil || saved == nil {
			return goja.Undefined()
		}

		return vm.ToValue(saved)
	})

	_ = autoSelectObj.Set("deleteProfile", func() goja.Value {
		database, ok := a.database.Get()
		if !ok {
			goja_bindings.PanicThrowErrorString(vm, "database not set")
		}

		if err := db_bridge.DeleteAutoSelectProfile(database); err != nil {
			goja_bindings.PanicThrowError(vm, err)
		}

		return goja.Undefined()
	})

	_ = obj.Set("autoSelect", autoSelectObj)
}
