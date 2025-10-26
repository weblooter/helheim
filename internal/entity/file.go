package entity

import "time"

type FileAction string

const (
	FileActionCreated = "C"
	FileActionUpdated = "U"
	FileActionDeleted = "D"
)

type File struct {
	Filepath     string
	LastModified time.Time
	Size         uint
	HashSum      string
}

type FilesStruct map[string]*File
