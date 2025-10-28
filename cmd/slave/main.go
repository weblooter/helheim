package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"weblooter/helheim/internal/grpc"
)

var flagSyncDir string
var flagPort uint
var flagDebugMode bool

func init() {
	flag.StringVar(&flagSyncDir, "dir", "", `Директория, которая подлежит синхронизации.`)
	flag.UintVar(&flagPort, "port", 50051, `Порт слушателя.`)
	flag.BoolVar(&flagDebugMode, "vv", false, `Включить режим дебага`)
	flag.Parse()
}

func main() {
	flagSyncDir = strings.TrimSpace(flagSyncDir)
	switch {
	case flagSyncDir == "":
		fmt.Println("Не задана директория для синхронизации. Используйте --help для получения детальной информации.")
		return
	case flagSyncDir == "." || flagSyncDir == "./" || flagSyncDir == ".." || flagSyncDir == "../":
		fmt.Println("Мы выступаем решильно против синхронизации от текущей директории (./) или от директории уровнем выше (../)")
		return
	}

	if flagDebugMode {
		log.Printf("Директория синхронизации: %v\n\n", flagSyncDir)
	}

	sigs := make(chan os.Signal, 1)
	exit := make(chan bool, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		exit <- true
	}()

	helheimServer, err := grpc.NewHelheimServer(flagPort, flagSyncDir, flagDebugMode)
	if err != nil {
		log.Fatal(err)
	}
	defer helheimServer.Defer()

	go func() {
		err = helheimServer.Run()
		if err != nil {
			log.Fatal(err)
		}
	}()

	<-exit
}
