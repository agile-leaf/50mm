package main

type Album struct {
	Path string

	AuthUser string
	AuthPass string

	BucketPrefix       string

	MetaTitle  string
	AlbumTitle string
}

func NewAlbumFromConfig(cfg )