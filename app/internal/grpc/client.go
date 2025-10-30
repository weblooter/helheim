package grpc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/weblooter/helheim/gen"
	"github.com/weblooter/helheim/internal/entity"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn      *grpc.ClientConn
	client    gen.HelheimClient
	chunkSize int64
}

// NewClient получить новый экземпляр клиента
func NewClient(addr string, chunkSize int) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc NewClient connect failed: %v", err)
	}

	g := &Client{
		conn:      conn,
		client:    gen.NewHelheimClient(conn),
		chunkSize: 1024 * 1024 * int64(chunkSize),
	}
	return g, nil
}

// UploadFile загрузка в получателя файла с признаком события
func (g *Client) UploadFile(scanDir string, action entity.FileAction, file entity.File) error {
	stream, err := g.client.UploadFile(context.Background())
	if err != nil {
		return fmt.Errorf("UploadFile open stream failed: %v", err)
	}
	// В зависимости от того, какое действие было запрошено,
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

		// Проверим нет ли проблем с файлом.
		f, err := os.Open(scanDir + file.Filepath)
		if err != nil {
			return fmt.Errorf("UploadFile open file failed: %v", err)
		}
		defer func() {
			_ = f.Close()
		}()
		fStat, err := f.Stat()
		if err != nil {
			return fmt.Errorf("UploadFile stat file failed: %v", err)
		}

		chunkMax := (fStat.Size() + g.chunkSize - 1) / g.chunkSize
		if chunkMax == 0 {
			chunkMax = 1
		}

		var reqAction gen.FileActionEnum
		switch {
		case action == entity.FileActionUpdated:
			reqAction = gen.FileActionEnum_ACTION_UPDATE
		default:
			reqAction = gen.FileActionEnum_ACTION_CREATE
		}

		// Начнем чтение файла и его отправку чанками
		buffer := make([]byte, g.chunkSize)
		chunkNum := int64(0)
		for {
			chunkNum++
			n, err := f.Read(buffer)
			if err == io.EOF {
				// Ошибка io.EOF обозначает, что мы достигли конца файла при чтении.
				// Но в случае, если файл сразу должен быть пустым (к примеру .gitkeep)
				// нам нужно отправить хотя бы 1 запрос в UploadFile,
				// иначе будет ошибка. Костыль ниже гарантирует, что запрос точно
				// будет отправлен.
				if chunkNum > 1 {
					break
				}
			} else if err != nil {
				return fmt.Errorf("UploadFile read file failed: %v", err)
			}

			chunkHash := fmt.Sprintf("%x", sha256.Sum256(buffer[:n]))

			req := &gen.UploadFileChunkRequest{
				Action:       reqAction,
				Filepath:     file.Filepath,
				ChunkContent: buffer[:n],
				ChunkNum:     chunkNum,
				ChunkMax:     chunkMax,
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

	if resp.Done != true {
		return fmt.Errorf("UploadFile \"%v\" resp error: file not done", file.Filepath)
	}

	return nil
}

// GetState получить состояние master
func (g *Client) GetState() (entity.FilesStruct, error) {
	req := &gen.GetStateRequest{}
	resp, err := g.client.GetState(context.Background(), req)
	if err != nil {
		return entity.FilesStruct{}, fmt.Errorf("client GetState call failed: %v", err)
	}

	state := entity.FilesStruct{}
	for h, f := range resp.Files {
		state[h] = &entity.File{
			Filepath: f.Filepath,
			HashSum:  f.ContentHashSum,
		}
	}
	return state, nil
}
