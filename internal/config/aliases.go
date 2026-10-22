package config

import "log/slog"

// aliases maps each grapi variable to the Strapi one it replaces.
var aliases = map[string]string{
	"GRAPI_ENV":                          "NODE_ENV",
	"GRAPI_ENV_PATH":                     "ENV_PATH",
	"GRAPI_LOG_LEVEL":                    "STRAPI_LOG_LEVEL",
	"GRAPI_TELEMETRY_DISABLED":           "STRAPI_TELEMETRY_DISABLED",
	"GRAPI_DISABLE_UPDATE_NOTIFICATION":  "STRAPI_DISABLE_UPDATE_NOTIFICATION",
	"GRAPI_HIDE_STARTUP_MESSAGE":         "STRAPI_HIDE_STARTUP_MESSAGE",
	"GRAPI_PLUGIN_I18N_INIT_LOCALE_CODE": "STRAPI_PLUGIN_I18N_INIT_LOCALE_CODE",
}

// Lookup reads a grapi variable, or its Strapi alias. The grapi name wins
// when both are set.
func (e Env) Lookup(name string, log *slog.Logger) (string, bool) {
	v, ok := e[name]
	alias, hasAlias := aliases[name]
	if !hasAlias {
		return v, ok
	}
	av, aok := e[alias]
	if ok && aok && av != v && log != nil {
		log.Warn("both " + name + " and " + alias + " are set, using " + name)
	}
	if ok {
		return v, true
	}
	return av, aok
}
