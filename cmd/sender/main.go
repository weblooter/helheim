package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
	"weblooter/helheim/internal/entity"
	"weblooter/helheim/internal/grpc"
	"weblooter/helheim/internal/service/scanner"

	"github.com/fatih/color"
)

var flagScanIntervalSec uint
var flagScanDir string
var flagServerAddr string
var flagDebugMode bool

func init() {
	flag.UintVar(&flagScanIntervalSec, "interval", 30, `Длительность перерыва между повторным сканированием структуры в секундах.
Чем реже меняется структура, тем длительней должен быть интервал.`)
	flag.StringVar(&flagScanDir, "dir", "", `Директория, которая подлежит сконированию и синхронизации.`)
	flag.StringVar(&flagServerAddr, "serverAddr", "127.0.0.1:50051", `Адрес получателя (gRPC server) в формате "HOST:PORT".`)
	flag.BoolVar(&flagDebugMode, "vv", false, `Включить режим дебага`)
	flag.Parse()
}

func main() {
	if flagScanIntervalSec == 0 {
		fmt.Println("Интервал не может быть короче 1 секунды.")
		return
	}
	flagScanDir = strings.TrimSpace(flagScanDir)
	switch {
	case flagScanDir == "":
		fmt.Println("Не задана директория для сканирования. Используйте --help для получения детальной информации.")
		return
	case flagScanDir == "." || flagScanDir == "./" || flagScanDir == ".." || flagScanDir == "../":
		fmt.Println("Мы выступаем решильно против скинирования от текущей директории (./) или от директории уровнем выше (../)")
		return
	}

	if flagDebugMode {
		log.Printf("Интервал сканирования: %v секунд.\n", flagScanIntervalSec)
		log.Printf("Директория синхронизации: %v\n\n", flagScanDir)
	}

	helheimClient, err := grpc.NewHelheimClient(flagServerAddr)
	if err != nil {
		log.Fatal(err)
	}

	scan := scanner.NewScanner(flagScanDir)
	scan.SetBaseState(make(entity.FilesStruct)) // TODO получить изменения с recipient и положить их как базовое состояние

	for {
		if flagDebugMode {
			log.Println(color.New(color.BgYellow, color.FgBlack).Sprint("=== NEW SCAN ==="))
		}

		// Запуск сканирования
		err = scan.Rescan()
		if err != nil {
			log.Fatal(err)
		}

		// Получим изменения
		changes := scan.GetChanges()
		for action, files := range changes {
			if len(files) > 0 {
				if flagDebugMode {
					log.Printf("Action: %s, Files: %v\n", action, len(files))
					for _, file := range files {
						col := &color.Color{}
						switch action {
						case entity.FileActionCreated:
							col = color.New(color.FgGreen)
						case entity.FileActionUpdated:
							col = color.New(color.FgYellow)
						case entity.FileActionDeleted:
							col = color.New(color.FgRed)
						}
						log.Println(col.Sprint(file.Filepath))
					}
				}

				for filepathHash, file := range files {

					err = helheimClient.UploadFile(flagScanDir, action, *file)
					if err != nil {
						log.Fatal(err)
					}

					err = scan.CommitFile(action, filepathHash)
					if err != nil {
						log.Fatal(err)
					}
					return // TODO удалить после дебага
				}
			}
		}

		time.Sleep(time.Duration(flagScanIntervalSec) * time.Second)
	}
}
