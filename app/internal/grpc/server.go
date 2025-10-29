package grpc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"weblooter/helheim/gen"
	"weblooter/helheim/internal/assistant/message"
	"weblooter/helheim/internal/service/scanner"

	"google.golang.org/grpc"
)

type Server struct {
	gen.UnimplementedHelheimServer
	server      *grpc.Server
	listener    net.Listener
	mu          sync.Mutex
	syncDir     string
	tmpDir      string
	isDebugMode bool
}

// NewServer создает экземпляр сервера
func NewServer(port uint, syncDir string, isDebugMode bool, chunkSize int) (*Server, error) {
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

	g := Server{
		server: grpc.NewServer(
			grpc.MaxRecvMsgSize((chunkSize+5)*1024*1024),
			grpc.MaxSendMsgSize((chunkSize+5)*1024*1024),
		),
		listener:    lis,
		syncDir:     syncDir,
		tmpDir:      tmpDir,
		isDebugMode: isDebugMode,
	}

	gen.RegisterHelheimServer(g.server, &g)

	return &g, nil
}

// Run запуск gRPC сервера
func (s *Server) Run() error {
	if err := s.server.Serve(s.listener); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

// Defer функция, которая удаляет временную директорию,
// используемую для временно размещения передаваемые файлы
func (s *Server) Defer() {
	_ = os.RemoveAll(s.tmpDir)
}

// UploadFile метод принятия загружаемый файлов от мастера
func (s *Server) UploadFile(stream gen.Helheim_UploadFileServer) error {
	var chunkFilepath *string
	var action *gen.FileActionEnum
	var tmpF *os.File
	var fileContentHashSum string
	tmpFileContentHash := sha256.New()
	var chunkMax int64
	var chunkNum int64
	defer func() {
		if tmpF != nil {
			_ = tmpF.Close()
			_ = os.Remove(tmpF.Name())
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// Пришел признак окончания стрима.
			if s.isDebugMode {
				fmt.Println()
			}
			switch {
			case *action == gen.FileActionEnum_ACTION_DELETE:
				// Стрим был на удаление. Удалим файл.
				f, err := os.Open(fmt.Sprintf("%s%s", s.syncDir, *chunkFilepath))
				if err != nil {
					message.ThrowFatal(err.Error())
					return err
				}
				err = os.Remove(f.Name())
				if err != nil {
					message.ThrowFatal(err.Error())
					return err
				}
				if s.isDebugMode {
					message.Info("Done\n")
				}

			case *action == gen.FileActionEnum_ACTION_CREATE || *action == gen.FileActionEnum_ACTION_UPDATE:
				// Стрим был на создание или обновление. Запишем файл.
				// Сверим корректность данных
				if chunkNum != chunkMax {
					err = fmt.Errorf("number of the last chunk is #%d, but it must be #%d", chunkNum, chunkMax)
					message.ThrowFatal(err.Error())
					return err
				}
				if fmt.Sprintf("%x", tmpFileContentHash.Sum(nil)) != fileContentHashSum {
					err = fmt.Errorf("file content hash does not match")
					message.ThrowFatal(err.Error())
					return err
				}

				finalFilepath := fmt.Sprintf("%s%s", s.syncDir, *chunkFilepath)
				if *action == gen.FileActionEnum_ACTION_CREATE {
					// Создадим файл
					if _, err := os.Stat(finalFilepath); os.IsExist(err) {
						err = fmt.Errorf("file %s already exists", *chunkFilepath)
						message.ThrowFatal(err.Error())
						return err
					}

					if _, err := os.Stat(filepath.Dir(finalFilepath)); os.IsNotExist(err) {
						if err := os.MkdirAll(filepath.Dir(finalFilepath), 0755); err != nil {
							err = fmt.Errorf("failed to create directory %s: %v", filepath.Dir(finalFilepath), err)
							message.ThrowFatal(err.Error())
							return err
						}
					}

					err := os.Rename(tmpF.Name(), finalFilepath)
					if err != nil {
						err = fmt.Errorf("rename file failed: %v", err)
						message.ThrowFatal(err.Error())
						return err
					}
					if s.isDebugMode {
						message.Info("Done\n")
					}
				} else if *action == gen.FileActionEnum_ACTION_UPDATE {
					// Обновим файл
					if _, err := os.Stat(finalFilepath); os.IsNotExist(err) {
						err = fmt.Errorf("file %s not exists", *chunkFilepath)
						message.ThrowFatal(err.Error())
						return err
					}

					// Откроем финальный файл (обнуляет если уже был создан)
					dstF, err := os.Create(finalFilepath)
					if err != nil {
						err = fmt.Errorf("create file failed: %v", err)
						message.ThrowFatal(err.Error())
						return err
					}
					defer func() {
						_ = dstF.Close()
					}()

					// Переместим курсор временного файла в начало
					_, err = tmpF.Seek(0, io.SeekStart)
					if err != nil {
						err = fmt.Errorf("seek file failed: %v", err)
						message.ThrowFatal(err.Error())
						return err
					}

					// Перемещаем содержимое
					_, err = io.Copy(dstF, tmpF)
					if err != nil {
						err = fmt.Errorf("copy file failed: %v", err)
						message.ThrowFatal(err.Error())
						return err
					}
					if s.isDebugMode {
						message.Info("Done\n")
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
		if chunkMax == 0 {
			chunkMax = req.ChunkMax
		}

		// Создадим временный файл для записи в него контента чанков
		if tmpF == nil {
			tmpF, err = os.CreateTemp(s.tmpDir, fmt.Sprintf("%s.*", req.FileHashSum))
			if err != nil {
				return err
			}
		}

		chunkNum = req.ChunkNum

		if s.isDebugMode {
			mgs := fmt.Sprintf("Filepath: %v, ChunkNum: %v, ChunkMax: %v", *chunkFilepath, req.ChunkNum, req.ChunkMax)
			switch *action {
			case gen.FileActionEnum_ACTION_DELETE:
				message.FlushDanger("[D] " + mgs)
			case gen.FileActionEnum_ACTION_UPDATE:
				message.FlushWarning("[U] " + mgs)
			case gen.FileActionEnum_ACTION_CREATE:
				message.FlushSuccess("[C] " + mgs)
			}
		}

		// Если приходят запросы на создание или обновление, то нужно
		//  записывать данные во временный файл
		if *action == gen.FileActionEnum_ACTION_CREATE || *action == gen.FileActionEnum_ACTION_UPDATE {

			// Сверим хэш сумму чанка
			chunkHash := fmt.Sprintf("%x", sha256.Sum256(req.ChunkContent))
			if chunkHash != req.ChunkHashSum {
				return fmt.Errorf("Chunk #%d hash sum not equil: %x != %x ", req.ChunkNum, chunkHash, req.ChunkHashSum)
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
func (s *Server) GetState(_ context.Context, _ *gen.GetStateRequest) (*gen.GetStateResponse, error) {
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
			ContentHashSum: f.HashSum,
		}
	}

	return &gen.GetStateResponse{Files: files}, nil
}
