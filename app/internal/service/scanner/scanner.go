package scanner

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"weblooter/helheim/internal/entity"
)

type Scanner struct {
	dir                string
	state              entity.FilesStruct
	lastChanges        map[entity.FileAction]entity.FilesStruct
	lastProcessedFiles []string
}

// NewScanner новый экземпляр
func NewScanner(dir string) Scanner {
	return Scanner{
		dir:   dir,
		state: make(entity.FilesStruct),
		lastChanges: map[entity.FileAction]entity.FilesStruct{
			entity.FileActionCreated: {},
			entity.FileActionUpdated: {},
			entity.FileActionDeleted: {},
		},
	}
}

// SetBaseState задает базовое состояние структуры.
// Подразумевается, что отправитель перед сканированием будет получать состояние получателя
// и записывать его через этот метод.
func (s *Scanner) SetBaseState(state entity.FilesStruct) {
	s.state = state
}

// GetState возвращает структуру
func (s *Scanner) GetState() entity.FilesStruct {
	return s.state
}

// Rescan запустить повторное сканирование
func (s *Scanner) Rescan() error {
	s.lastProcessedFiles = []string{}

	// Пройдемся по текущей структуре и найдем новые и измененные
	err := filepath.WalkDir(s.dir, s.rescanWalkDir)
	if err != nil {
		return err
	}

	// Найдем удаленные
	err = s.rescanFindRemoved()
	if err != nil {
		return err
	}

	return nil
}

// rescanWalkDir обрабатывает сканируемый файл, проверяя на изменения
func (s *Scanner) rescanWalkDir(path string, d fs.DirEntry, _ error) error {
	if !d.IsDir() {
		relativeFilepath := strings.TrimPrefix(path, s.dir)
		filepathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(relativeFilepath)))

		// Отметим файл в обработанных, что бы потом по ним найти удаленные
		s.lastProcessedFiles = append(s.lastProcessedFiles, filepathHash)

		// получим хэш сумму контента файла
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()
		hashSum := sha256.New()
		chunkSize := 1024 * 1024
		buffer := make([]byte, chunkSize)
		for {
			n, err := f.Read(buffer)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			hashSum.Write(buffer[:n])
		}

		// описываем файл
		researchFile := entity.File{
			Filepath: relativeFilepath,
			HashSum:  fmt.Sprintf("%x", hashSum.Sum(nil)),
		}

		// проверяем существование описанного файла в текущем состоянии
		if currentFileState, ok := s.state[filepathHash]; ok {
			// файл существует, проверим не изменился ли он
			if researchFile.HashSum != currentFileState.HashSum {
				// изменялся
				s.state[filepathHash] = &researchFile
				s.lastChanges[entity.FileActionUpdated][filepathHash] = &researchFile
			}
		} else {
			// новый файл
			s.state[filepathHash] = &researchFile
			s.lastChanges[entity.FileActionCreated][filepathHash] = &researchFile
		}

	}

	return nil
}

// rescanFindRemoved второй этап сканирования, выявляет удаленные файлы.
// Перемещенные файлы тоже считаются удаленными.
func (s *Scanner) rescanFindRemoved() error {
	// Сравним текущее состояние с обработанными файлам.
	for k, f := range s.state {
		if !slices.Contains(s.lastProcessedFiles, k) {
			// Файл из состояния не был обработан при последнем сканировании.
			// Это означает, что файл был удален.
			s.lastChanges[entity.FileActionDeleted][k] = f
		}
	}

	return nil
}

// GetChanges отдает измененные и удаленные файлы
func (s *Scanner) GetChanges() map[entity.FileAction]entity.FilesStruct {
	return s.lastChanges
}

// CommitFile фиксируем событие файла
func (s *Scanner) CommitFile(action entity.FileAction, filepathHash string) error {
	delete(s.lastChanges[action], filepathHash)
	// Если было событие удаления - удаляем файл из состояния
	if action == entity.FileActionDeleted {
		delete(s.state, filepathHash)
	}
	return nil
}
