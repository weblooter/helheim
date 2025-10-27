package grpc

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
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
	chunkSize   int64
	listener    net.Listener
	mu          sync.Mutex
	syncDir     string
	isDebugMode bool
}

// NewHelheimServer
func NewHelheimServer(port uint, syncDir string, isDebugMode bool) (*HelheimServer, error) {
	if _, err := os.Stat(syncDir); err != nil {
		return nil, fmt.Errorf("sync dir %s does not exist", syncDir)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	g := HelheimServer{
		server:      grpc.NewServer(),
		chunkSize:   1024 * 1024,
		listener:    lis,
		syncDir:     syncDir,
		isDebugMode: isDebugMode,
	}

	gen.RegisterHelheimServer(g.server, &g)

	return &g, nil
}

// Run
func (s *HelheimServer) Run() error {
	if err := s.server.Serve(s.listener); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

// UploadFile
func (s *HelheimServer) UploadFile(stream gen.Helheim_UploadFileServer) error {
	var filepath *string
	var action *gen.FileActionEnum

	for {
		req, err := stream.Recv()
		if err == io.EOF {

			switch {
			case *action == gen.FileActionEnum_ACTION_DELETE:
				// Поступил запрос на удаление файл.
				f, err := os.Open(fmt.Sprintf("%s%s", s.syncDir, *filepath))
				if err != nil {
					return err
				}
				err = os.Remove(f.Name())
				if err != nil {
					return err
				}
				if s.isDebugMode {
					log.Println(color.New(color.FgCyan).Sprintf("Файл \"%s\" удален.\n", *filepath))
				}

			case *action == gen.FileActionEnum_ACTION_CREATE || *action == gen.FileActionEnum_ACTION_UPDATE:
				// Послутил запрос на создание или обновление файла
				log.Println("=== CREATE || UPDATE")
			}

			return stream.SendAndClose(&gen.UploadFileResponse{
				Done: true,
			})

		} else if err != nil {
			return stream.SendAndClose(&gen.UploadFileResponse{
				Done:  false,
				Error: fmt.Sprintf("Stream error: %v", err),
			})
		}

		if filepath == nil && action == nil {
			filepath = &req.Filepath
			action = &req.Action
		}

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

			log.Println(col.Sprintf("Filepath: %v, ChunkIndex: %v, TotalChunks: %v", *filepath, req.ChunkIndex, req.TotalChunks))
		}
	}
}

// GetState собирает информацию о текущй структуре и передает отправителю
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
