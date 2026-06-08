package site

import "embed"

// StaticFiles contains the standalone site assets.
//
//go:embed static/*
var StaticFiles embed.FS
