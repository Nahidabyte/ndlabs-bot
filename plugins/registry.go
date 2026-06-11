package plugins

import (
	"wa-bot/database"
	"wa-bot/plugin"
)

var PluginRegistry []func(*plugin.Manager, *database.DB)

func RegisterAll(manager *plugin.Manager, db *database.DB) {

	for _, registerFunc := range PluginRegistry {
		registerFunc(manager, db)
	}

	for _, p := range plugin.WrapFunctionalPlugins(manager, db) {
		manager.Register(p)
	}
}
