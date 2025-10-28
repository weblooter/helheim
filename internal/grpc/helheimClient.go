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
		if err = stream.Send(req); err != nil {
			return fmt.Errorf("UploadFile \"%v\" failed to send chunk: %v", file.Filepath, err)
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

		totalChunks := (int64(file.Size) + g.chunkSize - 1) / g.chunkSize
		if totalChunks == 0 {
			// Костыль для файлов с пустым контентом, к примеру .gitkeep
			// Согласно логике у пустого файла кол-во батчей будет равно 0, но
			// первый отправленный батч помечается как #1, что вызывает ошибку
			// на стороне slave. Для этого и размещен тут этот костыль
			totalChunks++
		}

		var reqAction gen.FileActionEnum
		switch {
		case action == entity.FileActionUpdated:
			reqAction = gen.FileActionEnum_ACTION_UPDATE
		case action == entity.FileActionCreated:
			reqAction = gen.FileActionEnum_ACTION_CREATE
		}

		// Начнем чтение файла и его отправку батчами
		buffer := make([]byte, g.chunkSize)
		chunkIndex := int64(0)
		for {
			chunkIndex++
			n, err := f.Read(buffer)
			if err == io.EOF {
				// Ошибка io.EOF обозначает, что мы достигли конца файла при чтении.
				// Но в случае, если файл сразу должен быть пустым (к примеру
				// .gitkeep) нам нужно отправить хотя бы 1 запрос в UploadFile,
				// иначе будет ошибка. Костыль ниже гарантирует, что запрос точно
				// будет отправлен.
				if chunkIndex > 1 {
					break
				}
			} else if err != nil {
				return fmt.Errorf("UploadFile read file failed: %v", err)
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
			if err = stream.Send(req); err != nil {
				return fmt.Errorf("UploadFile \"%v\" failed to send chunk: %v", file.Filepath, err)
			}
		}
		break
	}

	// Закроем стрим и проверим результат на наличие ошибок
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("UploadFile \"%v\" close stream failed: %v", file.Filepath, err)
	}

	if resp.Error != "" {
		return fmt.Errorf("UploadFile \"%v\" resp error: %v", file.Filepath, resp.Error)
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
