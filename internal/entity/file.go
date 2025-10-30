package entity

type FileAction string

const (
	FileActionCreated = "C"
	FileActionUpdated = "U"
	FileActionDeleted = "D"
)

type File struct {
	Filepath string
	HashSum  string
}

type FilesStruct map[string]*File
