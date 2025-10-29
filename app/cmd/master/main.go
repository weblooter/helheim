package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"weblooter/helheim/internal/assistant/message"
	"weblooter/helheim/internal/entity"
	"weblooter/helheim/internal/grpc"
	"weblooter/helheim/internal/service/scanner"
)

var flagScanIntervalSec uint
var flagScanDir string
var flagAddr string
var flagChunkSize int
var flagDebugMode bool

func init() {
	flag.UintVar(&flagScanIntervalSec, "interval", 30, `Длительность перерыва между повторным сканированием структуры в секундах.
Чем реже меняется структура, тем длительней должен быть интервал.`)
	flag.StringVar(&flagScanDir, "dir", "", `Директория, которая подлежит сканированию.`)
	flag.StringVar(&flagAddr, "addr", "127.0.0.1:50051", `Адрес получателя (gRPC server) в формате "HOST:PORT".`)
	flag.BoolVar(&flagDebugMode, "vv", false, `Включить режим дебага`)
	flag.IntVar(&flagChunkSize, "chunkSize", 5, `Размер чанков в МБ при отправке в slave.`)
	flag.Parse()
}

func main() {
	if flagScanIntervalSec == 0 {
		message.ThrowFatal("Интервал не может быть короче 1 секунды.")
		os.Exit(1)
	}
	flagScanDir = strings.TrimSpace(flagScanDir)
	switch {
	case flagScanDir == "":
		message.ThrowFatal("Не задана директория для сканирования. Используйте --help для получения детальной информации.")
		os.Exit(1)
		return
	case flagScanDir == "." || flagScanDir == "./" || flagScanDir == ".." || flagScanDir == "../":
		message.ThrowFatal("Мы выступаем решительно против сканирования от текущей директории (./) или от директории уровнем выше (../)")
		os.Exit(1)
	}
	if flagChunkSize < 1 {
		message.ThrowFatal("Размер чанка не может быть менее 1 МБ")
		os.Exit(1)
	}

	if flagDebugMode {
		message.DebugF("Интервал сканирования: %v секунд.\n", flagScanIntervalSec)
		message.DebugF("Директория сканирования: %v\n", flagScanDir)
		message.DebugF("Размер чанка: %v МБ\n", flagChunkSize)
		fmt.Println()
	}

	client, err := grpc.NewClient(flagAddr, flagChunkSize)
	if err != nil {
		message.ThrowFatal(err.Error())
		os.Exit(1)
	}

	scan := scanner.NewScanner(flagScanDir)

	slaveState, err := client.GetState()
	if err != nil {
		message.ThrowFatal(err.Error())
		os.Exit(1)
	}
	scan.SetBaseState(slaveState)

	for {
		if flagDebugMode {
			fmt.Println()
			message.Debug("=== NEW SCAN ===")
			fmt.Println()
		}

		// Запуск сканирования
		err = scan.Rescan()
		if err != nil {
			message.ThrowFatal(err.Error())
			os.Exit(1)
		}

		// Получим изменения
		changes := scan.GetChanges()
		for action, files := range changes {
			if len(files) > 0 {
				if flagDebugMode {
					message.DebugF("Action: %s, Files: %v\n", action, len(files))
					for _, file := range files {
						switch action {
						case entity.FileActionCreated:
							message.Success(file.Filepath)
						case entity.FileActionUpdated:
							message.Warning(file.Filepath)
						case entity.FileActionDeleted:
							message.Danger(file.Filepath)
						}
						fmt.Println()
					}
				}

				for filepathHash, file := range files {

					err = client.UploadFile(flagScanDir, action, *file)
					if err != nil {
						message.ThrowFatal(err.Error())
						continue
					}

					err = scan.CommitFile(action, filepathHash)
					if err != nil {
						message.ThrowFatal(err.Error())
					}
				}
			}
		}

		time.Sleep(time.Duration(flagScanIntervalSec) * time.Second)
	}
}
