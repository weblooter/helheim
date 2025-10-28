package grpc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"weblooter/helheim/gen"
	"weblooter/helheim/internal/entity"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type HelheimClient struct {
	conn      *grpc.ClientConn
	client    gen.HelheimClient
	chunkSize int64
}

// NewHelheimClient получить новый экземпляр клиента
func NewHelheimClient(serverAddr string, butchSize int) (*HelheimClient, error) {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc NewHelheimClient connect failed: %v", err)
	}

	g := &HelheimClient{
		conn:      conn,
		client:    gen.NewHelheimClient(conn),
		chunkSize: 1024 * 1024 * int64(butchSize),
	}
	return g, nil
}

// UploadFile загрузка в получателя файла с признаком события
func (g *HelheimClient) UploadFile(scanDir string, action entity.FileAction, file entity.File) error {
	stream, err := g.client.UploadFile(context.Background())
	if err != nil {
		return fmt.Errorf("UploadFile open stream failed: %v", err)
	}
	// В зависимост от того, какое действие было запрошено,
	//  выберем сценарий работы
	switch {
	case action == entity.FileActionDeleted:
		// Запрос на удаление файла. Отправим пустые данные
		//  с признаком удаления.
		req := &gen.UploadFileChunkRequest{
			Action:   gen.FileActionEnum_ACTION_DELETE,
			Filepath: file.Filepath,
		}
		if err := stream.Send(req); err != nil {
			return fmt.Errorf("UploadFile failed to send chunk: %v", err)
		}
		break
	case action == entity.FileActionCreated || action == entity.FileActionUpdated:
		// Запрос на обновление или создание файла.
		// Подготовим данные и отправим запросы по стриму.

		// Проверм нет ли пробем с файлом.
		f, err := os.Open(scanDir + file.Filepath)
		if err != nil {
			return fmt.Errorf("UploadFile open file failed: %v", err)
		}
		defer f.Close()

		// Подготовим общие данные для отправки
		totalChunks := (int64(file.Size) + g.chunkSize - 1) / g.chunkSize
		var reqAction gen.FileActionEnum
		switch action {
		case entity.FileActionCreated:
			reqAction = gen.FileActionEnum_ACTION_CREATE
		case entity.FileActionUpdated:
			reqAction = gen.FileActionEnum_ACTION_UPDATE
		}

		// Начнем чтение файла и его отправку батчами
		buffer := make([]byte, g.chunkSize)
		chunkIndex := int64(0)
		for {
			chunkIndex++
			n, err := f.Read(buffer)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			chunkHash := fmt.Sprintf("%x", sha256.Sum256(buffer[:n]))

			req := &gen.UploadFileChunkRequest{
				Action:       reqAction,
				Filepath:     file.Filepath,
				Size:         int64(file.Size),
				ChunkContent: buffer[:n],
				ChunkIndex:   chunkIndex,
				TotalChunks:  totalChunks,
				ChunkHashSum: chunkHash,
				FileHashSum:  file.HashSum,
			}
			if err := stream.Send(req); err != nil {
				return fmt.Errorf("UploadFile failed to send chunk: %v", err)
			}
		}
		break
	}

	// Закроем стрим и проверим результат на наличие ошибок
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("UploadFile close stream failed: %v", err)
	}

	if resp.Error != "" {
		return fmt.Errorf("UploadFile resp error: %v", resp.Error)
	}

	return nil
}

// GetState
func (g *HelheimClient) GetState() (entity.FilesStruct, error) {
	req := &gen.GetStateRequest{}
	resp, err := g.client.GetState(context.Background(), req)
	if err != nil {
		return entity.FilesStruct{}, fmt.Errorf("HelheimClient GetState call failed: %v", err)
	}

	state := entity.FilesStruct{}
	for h, f := range resp.Files {
		state[h] = &entity.File{
			Filepath:     f.Filepath,
			Size:         uint(f.Size),
			LastModified: f.LastModified.AsTime().UTC(),
			HashSum:      f.ContentHashSum,
		}
	}
	return state, nil
}
