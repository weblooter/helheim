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
var flagButchSize int

func init() {
	flag.StringVar(&flagSyncDir, "dir", "", `Директория, которая подлежит синхронизации.`)
	flag.UintVar(&flagPort, "port", 50051, `Порт слушателя.`)
	flag.BoolVar(&flagDebugMode, "vv", false, `Включить режим дебага`)
	flag.IntVar(&flagButchSize, "butchSize", 5, `Максимальный размер батчей от master в МБ.`)
	flag.Parse()
}

func main() {
	flagSyncDir = strings.TrimSpace(flagSyncDir)
	switch {
	case flagSyncDir == "":
		fmt.Fprintln(os.Stderr, "Не задана директория для синхронизации. Используйте --help для получения детальной информации.")
		os.Exit(1)
	case flagSyncDir == "." || flagSyncDir == "./" || flagSyncDir == ".." || flagSyncDir == "../":
		fmt.Fprintln(os.Stderr, "Мы выступаем решильно против синхронизации от текущей директории (./) или от директории уровнем выше (../)")
		os.Exit(1)
	}
	if flagButchSize < 1 {
		fmt.Fprintln(os.Stderr, "Размер батча не может быть менее 1 МБ")
		os.Exit(1)
	}

	if flagDebugMode {
		log.Printf("Директория синхронизации: %v\n", flagSyncDir)
		log.Printf("Размер батча: %v МБ (выделим gRPC серверу +5МБ)\n\n", flagButchSize)
	}

	sigs := make(chan os.Signal, 1)
	exit := make(chan bool, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		exit <- true
	}()

	helheimServer, err := grpc.NewHelheimServer(flagPort, flagSyncDir, flagDebugMode, flagButchSize)
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
