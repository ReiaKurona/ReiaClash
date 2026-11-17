// Package version 保存构建时显示的版本信息。
package version

// Agent 在正式构建时可由 -ldflags 覆盖；本地开发构建使用 dev。
var Agent = "dev"
