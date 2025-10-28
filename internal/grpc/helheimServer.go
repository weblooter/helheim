package grpc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"weblooter/helheim/gen"
	"weblooter/helheim/internal/service/scanner"

	"github.com/fatih/color"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HelheimServer struct {
	gen.UnimplementedHelheimServer
	server      *grpc.Server
	listener    net.Listener
	mu          sync.Mutex
	syncDir     string
	tmpDir      string
	isDebugMode bool
}

// NewHelheimServer создает экземпляр сервера
func NewHelheimServer(port uint, syncDir string, isDebugMode bool) (*HelheimServer, error) {
	if _, err := os.Stat(syncDir); err != nil {
		return nil, fmt.Errorf("sync dir %s does not exist", syncDir)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	tmpDir, err := os.MkdirTemp(syncDir, "~temp_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %v", err)
	}

	g := HelheimServer{
		server:      grpc.NewServer(),
		listener:    lis,
		syncDir:     syncDir,
		tmpDir:      tmpDir,
		isDebugMode: isDebugMode,
	}

	gen.RegisterHelheimServer(g.server, &g)

	return &g, nil
}

// Run запуск gRPC сервера
func (s *HelheimServer) Run() error {
	if err := s.server.Serve(s.listener); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

// Defer функция, которая удаляте временную директорию,
// в которую складывались передаваемые файлы
func (s *HelheimServer) Defer() {
	os.RemoveAll(s.tmpDir)
}

// UploadFile метод принятия загружаемый файлов от мастера
func (s *HelheimServer) UploadFile(stream gen.Helheim_UploadFileServer) error {
	var chunkFilepath *string
	var action *gen.FileActionEnum
	var tmpF *os.File
	var fileContentHashSum string
	tmpFileContentHash := sha256.New()
	var totalChunks int64
	var lastChunkIndex int64

	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// Пришел признак окончания стрима.
			switch {
			case *action == gen.FileActionEnum_ACTION_DELETE:
				// Стрим был на удаление. Удалим файл.
				f, err := os.Open(fmt.Sprintf("%s%s", s.syncDir, *chunkFilepath))
				if err != nil {
					return err
				}
				err = os.Remove(f.Name())
				if err != nil {
					return err
				}
				if s.isDebugMode {
					log.Println(color.New(color.FgCyan).Sprintf("Файл \"%s\" удален.\n", *chunkFilepath))
				}

			case *action == gen.FileActionEnum_ACTION_CREATE || *action == gen.FileActionEnum_ACTION_UPDATE:
				// Стрим был на создание или обновление. Запишем файл.
				defer tmpF.Close()

				// Сверим корректность данных
				if lastChunkIndex != totalChunks {
					return fmt.Errorf("number of the last chunk is #%d, but it must be #%d", lastChunkIndex, totalChunks)
				}
				if fmt.Sprintf("%x", tmpFileContentHash.Sum(nil)) != fileContentHashSum {
					return fmt.Errorf("file content hash sum does not match")
				}

				finalFilepath := fmt.Sprintf("%s%s", s.syncDir, *chunkFilepath)
				if *action == gen.FileActionEnum_ACTION_CREATE {
					// Создадим файл
					if _, err := os.Stat(finalFilepath); os.IsExist(err) {
						return fmt.Errorf("file %s already exists", *chunkFilepath)
					}

					if _, err := os.Stat(filepath.Dir(finalFilepath)); os.IsNotExist(err) {
						if err := os.MkdirAll(filepath.Dir(finalFilepath), 0755); err != nil {
							return fmt.Errorf("failed to create directory %s: %v", filepath.Dir(finalFilepath), err)
						}
					}

					err := os.Rename(tmpF.Name(), finalFilepath)
					if err != nil {
						return fmt.Errorf("rename file failed: %v", err)
					}
					if s.isDebugMode {
						log.Println(color.New(color.FgCyan).Sprintf("Файл \"%s\" создан.\n", *chunkFilepath))
					}
				} else if *action == gen.FileActionEnum_ACTION_UPDATE {
					// Обновим файл
					if _, err := os.Stat(finalFilepath); os.IsNotExist(err) {
						return fmt.Errorf("file %s not exists", *chunkFilepath)
					}

					// Откроем финальный файл (обнуляет если уже был создан)
					dstF, err := os.Create(finalFilepath)
					if err != nil {
						return fmt.Errorf("create file failed: %v", err)
					}
					defer dstF.Close()

					// Переместим курсор временного файла в начало
					_, err = tmpF.Seek(0, io.SeekStart)
					if err != nil {
						return fmt.Errorf("seek file failed: %v", err)
					}

					// Перемещаем содержимое
					_, err = io.Copy(dstF, tmpF)
					if err != nil {
						return fmt.Errorf("copy file failed: %v", err)
					}
					if s.isDebugMode {
						log.Println(color.New(color.FgCyan).Sprintf("Файл \"%s\" обновлен.\n", *chunkFilepath))
					}

				}
			}

			return stream.SendAndClose(&gen.UploadFileResponse{
				Done: true,
			})

		} else if err != nil {
			return err
		}

		// Запомним данные при первом обращении
		if chunkFilepath == nil {
			chunkFilepath = &req.Filepath
		}
		if action == nil {
			action = &req.Action
		}
		if fileContentHashSum == "" {
			fileContentHashSum = req.FileHashSum
		}
		if totalChunks == 0 {
			totalChunks = req.TotalChunks
		}

		// Создадим временный файл для записи в него контента чанков
		if tmpF == nil {
			tmpF, err = os.CreateTemp(s.tmpDir, fmt.Sprintf("%s.*", req.FileHashSum))
			if err != nil {
				return err
			}
			defer os.Remove(tmpF.Name())
		}

		lastChunkIndex = req.ChunkIndex

		if s.isDebugMode {
			col := &color.Color{}
			switch *action {
			case gen.FileActionEnum_ACTION_DELETE:
				col = color.New(color.FgRed)
			case gen.FileActionEnum_ACTION_UPDATE:
				col = color.New(color.FgYellow)
			case gen.FileActionEnum_ACTION_CREATE:
				col = color.New(color.FgGreen)
			}

			log.Println(col.Sprintf("Filepath: %v, ChunkIndex: %v, TotalChunks: %v", *chunkFilepath, req.ChunkIndex, req.TotalChunks))
		}

		// Если приходят запросы на создание или обновление, то нужну
		//  записывать данные во временный файл
		if *action == gen.FileActionEnum_ACTION_CREATE || *action == gen.FileActionEnum_ACTION_UPDATE {

			// Сверим хэш сумму чанка
			chunkHash := fmt.Sprintf("%x", sha256.Sum256(req.ChunkContent))
			if chunkHash != req.ChunkHashSum {
				return fmt.Errorf("Chink #%d hash sum not equil: %x != %x ", req.ChunkIndex, chunkHash, req.ChunkHashSum)
			}

			tmpFileContentHash.Write(req.ChunkContent)

			_, err := tmpF.Write(req.ChunkContent)
			if err != nil {
				return err
			}
		}
	}
}

// GetState собирает информацию о текущей структуре для мастера
func (s *HelheimServer) GetState(_ context.Context, _ *gen.GetStateRequest) (*gen.GetStateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files := make(map[string]*gen.GetStateResponseItem)

	scan := scanner.NewScanner(s.syncDir)
	err := scan.Rescan()
	if err != nil {
		return nil, fmt.Errorf("failed to scan: %v", err)
	}
	for h, f := range scan.GetState() {
		files[h] = &gen.GetStateResponseItem{
			Filepath:       f.Filepath,
			Size:           int64(f.Size),
			ContentHashSum: f.HashSum,
			LastModified:   timestamppb.New(f.LastModified),
		}
	}

	return &gen.GetStateResponse{Files: files}, nil
}
